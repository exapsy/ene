package httpunit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/joho/godotenv"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gopkg.in/yaml.v3"

	"github.com/exapsy/ene/e2eframe"
	"github.com/exapsy/ene/e2eframe/ui"
)

// UserFriendlyError interface for errors that can provide simplified messages
type UserFriendlyError interface {
	UserFriendlyMessage() string
}

type EnvFileLoadingError struct {
	path string
	err  error
}

func (e *EnvFileLoadingError) Error() string {
	return fmt.Sprintf("failed to load env file: %s", e.path)
}

func (e *EnvFileLoadingError) UserFriendlyMessage() string {
	return "could not find env file"
}

func (e *EnvFileLoadingError) Unwrap() error {
	return e.err
}

type Logs *string

func NewLogs(logs *string) Logs {
	return Logs(logs)
}

// ContainerCreationError represents a failure during container creation/build.
// When this error occurs, detailed logs are automatically saved to a file in
// .ene/logs/ directory for troubleshooting, and the file path is included in
// the error message.
type ContainerCreationError struct {
	containerName string
	err           error
	logs          Logs
	verbose       bool
	logFilePath   string // Path to the saved error log file
}

func (e *ContainerCreationError) Error() string {
	return fmt.Sprintf("failed to create container %s: %v", e.containerName, e.err)
}

func (e *ContainerCreationError) UserFriendlyMessage() string {
	errMsg := e.err.Error()
	prefix := fmt.Sprintf("%s build failed", e.containerName)

	var out strings.Builder

	// Determine the main error message and suggestions
	var mainError string
	var suggestions []string

	// Provide helpful hints for common errors
	if strings.Contains(errMsg, "not found") && strings.Contains(errMsg, ".netrc") {
		mainError = ".netrc file not found"
		suggestions = []string{
			"Required for private Go modules",
			"Create .netrc in project root with credentials",
			"See project README for setup instructions",
		}
	} else if strings.Contains(errMsg, "not found") && (strings.Contains(errMsg, "go.mod") || strings.Contains(errMsg, "go.sum")) {
		mainError = "go.mod or go.sum not found"
		suggestions = []string{
			"Check Dockerfile context is set correctly",
			"Verify Dockerfile path in suite.yml",
		}
	} else if strings.Contains(errMsg, "exit code: 1") && strings.Contains(errMsg, "go mod download") {
		mainError = "go mod download failed"
		suggestions = []string{
			"Check .netrc file has valid credentials",
			"Verify network connectivity",
			"Try: docker system prune to clear build cache",
		}
	} else if strings.Contains(errMsg, "Dockerfile") && strings.Contains(errMsg, "not found") {
		mainError = "Dockerfile not found"
		suggestions = []string{
			"Check the dockerfile path in your suite.yml",
			"Path should be relative to the suite directory",
		}
	} else if strings.Contains(errMsg, "Cannot connect to the Docker daemon") {
		// Docker daemon issues
		mainError = "cannot connect to Docker"
		suggestions = []string{
			"Check if Docker is running",
			"Try: colima start (macOS) / systemctl start docker (Linux)",
		}
	} else if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "context deadline exceeded") {
		// Build timeout or resource issues
		mainError = "build timeout"
		suggestions = []string{
			"Build is taking too long",
			"Check for network issues or large dependencies",
			"Consider increasing build_timeout in suite.yml",
		}
	} else {
		// For other errors, show a truncated version
		if len(errMsg) > 100 {
			mainError = errMsg[:97] + "..."
		} else {
			mainError = errMsg
		}
		suggestions = []string{
			"Check Docker daemon logs for full build output",
			"Run with --debug to see build output in real-time",
			"Try: docker system prune to clear cache",
		}
	}

	// Build the error message
	out.WriteString(prefix)
	out.WriteString(": ")
	out.WriteString(mainError)

	// Add suggestions
	for _, suggestion := range suggestions {
		out.WriteString("\n  → ")
		out.WriteString(suggestion)
	}

	// Add log file location if available
	if e.logFilePath != "" {
		out.WriteString("\n  → Full logs saved to: ")
		out.WriteString(e.logFilePath)
	}

	// Add container logs if verbose mode is enabled
	if e.verbose && e.logs != nil && *e.logs != "" {
		logBoxConfig := ui.DefaultLogBoxConfig()
		logBoxConfig.Title = "Container logs:"
		logBoxConfig.Indent = 2 // Align with error icon
		logBoxConfig.MaxWidth = 80
		logBoxConfig.HighlightErrors = true

		out.WriteString(ui.LogBox(*e.logs, logBoxConfig))
	}

	return out.String()
}

func (e *ContainerCreationError) Unwrap() error {
	return e.err
}

type PortAllocationError struct {
	err error
}

func (e *PortAllocationError) Error() string {
	return fmt.Sprintf("failed to allocate port: %v", e.err)
}

func (e *PortAllocationError) UserFriendlyMessage() string {
	return "no available ports"
}

func (e *PortAllocationError) Unwrap() error {
	return e.err
}

type DockerfileNotFoundError struct {
	path string
	err  error
}

func (e *DockerfileNotFoundError) Error() string {
	return fmt.Sprintf("dockerfile not found: %s", e.path)
}

func (e *DockerfileNotFoundError) UserFriendlyMessage() string {
	return "dockerfile not found"
}

func (e *DockerfileNotFoundError) Unwrap() error {
	return e.err
}

const (
	UnitKind e2eframe.UnitKind = "http"
)

const (
	DefaultBuildTimeout   = 45 * time.Second
	DefaultStartupTimeout = 30 * time.Second
)

type HTTPUnitConfig struct {
	Name            string        `yaml:"name"`
	Command         []string      `yaml:"command"`
	Dockerfile      string        `yaml:"dockerfile"`
	Image           string        `yaml:"image"`
	AppPort         int           `yaml:"app_port"`
	HealthcheckPath string        `yaml:"healthcheck"`
	EnvFile         string        `yaml:"env_file"`
	Env             any           `yaml:"env"`
	BuildTimeout    time.Duration `yaml:"build_timeout"`
	StartupTimeout  time.Duration `yaml:"startup_timeout"`
}

type HTTPUnit struct {
	Network         *testcontainers.DockerNetwork
	name            string
	Command         []string
	Dockerfile      string
	Image           string
	AppPort         int
	Port            int
	HealthcheckPath string
	EnvFile         string
	EnvVars         map[string]any
	cont            testcontainers.Container
	BuildTimeout    time.Duration
	StartupTimeout  time.Duration
	endpoint        string
	verbose         bool

	// Build optimization
	buildSemaphore chan struct{}

	// Logs from the service
	logs       []string
	logLock    sync.Mutex
	buildLogs  strings.Builder
	buildMutex sync.Mutex

	// File path for error logs
	errorLogFile string
}

func init() {
	e2eframe.RegisterUnitMarshaller(UnitKind, UnmarshallUnit)
}

func New(cfg map[string]any) (e2eframe.Unit, error) {
	name, ok := cfg["name"].(string)
	if !ok {
		return nil, fmt.Errorf("http plugin requires 'name'")
	}

	cmdIface, ok := cfg["command"].(any)
	if !ok {
		return nil, fmt.Errorf("http plugin requires 'command'")
	}

	appPort, ok := cfg["app_port"].(int)
	if !ok {
		return nil, fmt.Errorf("http plugin requires 'app_port'")
	}

	if appPort <= 0 {
		return nil, fmt.Errorf("http plugin requires 'app_port' to be greater than 0")
	}

	var cmd []string
	switch cmdIface := cmdIface.(type) {
	case []any:
		cmd = make([]string, len(cmdIface))

		for i, v := range cmdIface {
			switch v := v.(type) {
			case string:
				cmd[i] = v
			default:
				return nil, fmt.Errorf("http plugin requires 'command' to be a list of strings")
			}
		}
	case []string:
		cmd = cmdIface
	case string:
		// break string into cmd and args
		cmds := strings.Split(cmdIface, " ")
		cmd = make([]string, len(cmds))

		for i, v := range cmds {
			cmd[i] = v
		}
	default:
		return nil, fmt.Errorf("http plugin requires 'command' to be a list of strings")
	}

	dockerfile, ok := cfg["dockerfile"].(string)
	if !ok {
		return nil, fmt.Errorf("http plugin requires 'dockerfile'")
	}

	image, ok := cfg["image"].(string)
	if !ok {
		image = ""
	}

	if image == "" && dockerfile == "" {
		return nil, fmt.Errorf("http plugin requires 'image' or 'dockerfile'")
	}

	if image != "" && dockerfile != "" {
		return nil, fmt.Errorf("http plugin requires 'image' or 'dockerfile', not both")
	}

	envFile, _ := cfg["env_file"].(string)
	healthcheck, _ := cfg["healthcheck"].(string)

	buildTimeout, ok := cfg["build_timeout"].(time.Duration)
	if !ok {
		buildTimeout = DefaultBuildTimeout
	}

	if buildTimeout < 0 {
		return nil, fmt.Errorf("http plugin requires 'build_timeout' to be greater than 0")
	}

	if buildTimeout == 0 {
		buildTimeout = DefaultBuildTimeout
	}

	startupTimeout, ok := cfg["startup_timeout"].(time.Duration)
	if !ok {
		startupTimeout = DefaultStartupTimeout
	}

	if startupTimeout < 0 {
		return nil, fmt.Errorf("http plugin requires 'startup_timeout' to be greater than 0")
	}

	if startupTimeout == 0 {
		startupTimeout = DefaultStartupTimeout
	}

	envVarsAny, ok := cfg["env"].([]interface{})
	if !ok {
		envVarsAny = nil
	}

	envVars := make(map[string]any)

	if envVarsAny != nil {
		for _, v := range envVarsAny {
			switch v := v.(type) {
			case string:
				// split into key and value
				parts := strings.SplitN(v, "=", 2)
				if len(parts) != 2 {
					return nil, fmt.Errorf("http plugin requires 'env' to be a list of strings in the format 'key=value'")
				}

				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				envVars[key] = value
			default:
				return nil, fmt.Errorf("http plugin requires 'env' to be a list of strings in the format 'key=value'")
			}
		}
	}

	// Initialize performance optimizations with sensible defaults
	maxConcurrentBuilds := runtime.NumCPU()
	if maxConcurrentBuilds < 1 {
		maxConcurrentBuilds = 1
	}
	buildSemaphore := make(chan struct{}, maxConcurrentBuilds)

	return &HTTPUnit{
		name:            name,
		Command:         cmd,
		EnvFile:         envFile,
		EnvVars:         envVars,
		AppPort:         appPort,
		HealthcheckPath: healthcheck,
		Dockerfile:      dockerfile,
		Image:           image,
		BuildTimeout:    buildTimeout,
		StartupTimeout:  startupTimeout,
		buildSemaphore:  buildSemaphore,
	}, nil
}

func (s *HTTPUnit) Name() string {
	return s.name
}

// Start starts the HTTP unit in a docker container,
// runs the command specified in the config,
// and exposes the port to the host.
func (s *HTTPUnit) Start(ctx context.Context, opts *e2eframe.UnitStartOptions) error {
	// load env file into map
	envs := make(map[string]string)

	var err error
	if s.EnvFile != "" {
		envFile := filepath.Join(opts.WorkingDir, s.EnvFile)
		if err := godotenv.Load(envFile); err != nil {
			return &EnvFileLoadingError{
				path: envFile,
				err:  err,
			}
		}

		envs, err = godotenv.Read(envFile)
		if err != nil {
			return &EnvFileLoadingError{
				path: envFile,
				err:  err,
			}
		}
	}

	// load env vars from config
	if s.EnvVars != nil {
		for key, value := range s.EnvVars {
			switch v := value.(type) {
			case string:
				envs[key] = v
			case int:
				envs[key] = fmt.Sprintf("%d", v)
			case bool:
				envs[key] = fmt.Sprintf("%t", v)
			default:
				return fmt.Errorf("unsupported env var type: %T", v)
			}
		}
	}

	// Resolve the Dockerfile path relative to WorkingDir (suite directory)
	dockerfilePath := filepath.Join(opts.WorkingDir, s.Dockerfile)
	dockerfilePath, err = filepath.Abs(dockerfilePath)
	if err != nil {
		return fmt.Errorf("get absolute path of dockerfile: %w", err)
	}

	// The build context is the directory containing the Dockerfile
	dockerfileDir := filepath.Dir(dockerfilePath)

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	dockerBaseDir, err := filepath.Rel(cwd, dockerfileDir)
	if err != nil {
		return fmt.Errorf("get relative path of dockerfile directory: %w", err)
	}

	var buildLogWriter io.Writer

	var logConsumers []testcontainers.LogConsumer

	logCapture := &httpLogConsumer{unit: s}

	// Create log file for capturing build output
	// Use project root .ene/<suite-name>/ directory
	logDir := filepath.Join(".ene", opts.SuiteName)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Printf("Warning: failed to create log directory: %v\n", err)
	}

	timestamp := time.Now().Format("20060102-150405")
	s.errorLogFile = filepath.Join(logDir, fmt.Sprintf("build-%s-%s.log", s.name, timestamp))
	logFile, err := os.Create(s.errorLogFile)
	if err != nil {
		fmt.Printf("Warning: failed to create log file: %v\n", err)
		logFile = nil
	}

	// Capture build logs for error reporting in verbose mode
	if opts.Verbose || opts.Debug {
		if logFile != nil {
			buildLogWriter = io.MultiWriter(os.Stdout, &s.buildLogs, logFile)
		} else {
			buildLogWriter = io.MultiWriter(os.Stdout, &s.buildLogs)
		}
		logConsumers = []testcontainers.LogConsumer{
			logCapture,
		}
	} else {
		if logFile != nil {
			buildLogWriter = io.MultiWriter(&s.buildLogs, logFile)
			defer logFile.Close()
		} else {
			buildLogWriter = &s.buildLogs
		}
		logConsumers = []testcontainers.LogConsumer{logCapture}
	}

	// Create a buffer to capture container runtime logs
	var runtimeLogBuffer strings.Builder
	runtimeLogMutex := &sync.Mutex{}
	runtimeLogCapture := &containerLogCapture{
		buffer: &runtimeLogBuffer,
		mutex:  runtimeLogMutex,
	}
	if !opts.Verbose && !opts.Debug {
		logConsumers = append(logConsumers, runtimeLogCapture)
	}

	// Cleanup old cached images if requested
	if opts.CleanupCache {
		if err := s.cleanupOldCachedImages(ctx); err != nil {
			// Log warning but don't fail the test
			fmt.Printf("Warning: failed to cleanup old cached images: %v\n", err)
		}
	}

	var fromDockerfile testcontainers.FromDockerfile
	var image string

	if s.Dockerfile != "" {
		// Acquire build semaphore to limit concurrent builds
		s.buildSemaphore <- struct{}{}
		defer func() { <-s.buildSemaphore }()

		// Generate smart content-based tag for better cache behavior
		contentHash, err := s.generateSmartContentHash(dockerBaseDir)
		if err != nil {
			return fmt.Errorf("failed to generate smart content hash: %w", err)
		}

		imageName := fmt.Sprintf("ene-%s:%s", s.name, contentHash)

		// Check if image already exists to skip rebuild
		if opts.CacheImages && s.imageExists(ctx, imageName) {
			// Image exists, use it directly instead of rebuilding
			image = imageName

			s.sendEvent(
				opts.EventSink,
				e2eframe.EventContainerStarting,
				fmt.Sprintf("using cached image for HTTP unit %s", s.Name()),
			)
		} else {
			// Image doesn't exist or caching disabled, build it
			fromDockerfile = testcontainers.FromDockerfile{
				Context:        dockerBaseDir,
				Dockerfile:     filepath.Base(dockerfilePath),
				BuildLogWriter: buildLogWriter,
				Repo:           fmt.Sprintf("ene-%s", s.name),
				Tag:            contentHash,
				BuildOptionsModifier: func(buildOptions *types.ImageBuildOptions) {
					// Use legacy builder for proper log capture
					// BuildKit doesn't stream logs to BuildLogWriter properly
					buildOptions.Version = types.BuilderV1
					buildOptions.BuildArgs = map[string]*string{}

					// Add no-cache option for debugging only
					if opts.Debug {
						buildOptions.NoCache = true
					}
				},
			}
			if opts.CacheImages {
				fromDockerfile.KeepImage = true
			}
		}
	} else if s.Image != "" {
		image = s.Image
	}

	// Create host port
	freePort, err := e2eframe.GetFreePort()
	if err != nil {
		return &PortAllocationError{err: err}
	}

	s.Port = freePort
	exposedPort := fmt.Sprintf("%d", freePort)
	// exposedPortNat := nat.Port(fmt.Sprintf("%s/tcp", exposedPort))

	// appPortStr := fmt.Sprintf("%d", s.AppPort)
	appPortStrNat := fmt.Sprintf("%d/tcp", s.AppPort)
	appPortNat := nat.Port(appPortStrNat)

	req := testcontainers.ContainerRequest{
		Image:          image,
		FromDockerfile: fromDockerfile,
		HostConfigModifier: func(hostConfig *container.HostConfig) {
			// Use sensible resource limits (512MB memory, 0.5 CPU)
			memoryBytes := int64(512 * 1024 * 1024)
			hostConfig.Memory = memoryBytes
			hostConfig.MemorySwap = memoryBytes // no additional swap

			// Set CPU limits
			hostConfig.CPUQuota = int64(0.5 * 100000)
			hostConfig.CPUPeriod = 100000

			// Optimize for faster startup
			hostConfig.AutoRemove = true

			hostConfig.PortBindings = map[nat.Port][]nat.PortBinding{
				appPortNat: {
					{
						HostIP:   "0.0.0.0",
						HostPort: exposedPort,
					},
				},
			}
		},
		ExposedPorts: []string{appPortStrNat},
		Env:          envs,
		Cmd:          s.Command,
		LogConsumerCfg: &testcontainers.LogConsumerConfig{
			Consumers: logConsumers,
		},
		Networks: []string{opts.Network.Name},
		NetworkAliases: map[string][]string{
			opts.Network.Name: {s.name},
		},
		WaitingFor: wait.
			ForHTTP(s.HealthcheckPath).
			WithPort(appPortNat).
			WithStartupTimeout(s.StartupTimeout).
			WithPollInterval(100 * time.Millisecond), // Faster polling for quicker detection
	}

	s.sendEvent(
		opts.EventSink,
		e2eframe.EventContainerStarting,
		fmt.Sprintf("building and starting HTTP unit %s on port %d", s.Name(), s.Port),
	)

	// Create a context with build timeout for the container creation/build phase
	buildCtx, buildCancel := context.WithTimeout(ctx, s.BuildTimeout)
	defer buildCancel()

	cont, err := testcontainers.GenericContainer(buildCtx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		// Always write error information to log file for troubleshooting
		s.buildMutex.Lock()
		buildLogStr := s.buildLogs.String()
		s.buildMutex.Unlock()

		// Try to get container runtime logs if container was created
		if cont != nil {
			logReader, logErr := cont.Logs(ctx)
			if logErr == nil && logReader != nil {
				defer logReader.Close()
				logBytes, readErr := io.ReadAll(logReader)
				if readErr == nil && len(logBytes) > 0 {
					if buildLogStr != "" {
						buildLogStr += "\n=== Container Runtime Logs ===\n"
					}
					buildLogStr += string(logBytes)
				}
			}
		}

		// Also add captured runtime logs from the log consumer
		runtimeLogMutex.Lock()
		runtimeLogs := runtimeLogBuffer.String()
		runtimeLogMutex.Unlock()
		if runtimeLogs != "" {
			if buildLogStr != "" {
				buildLogStr += "\n=== Application Output ===\n"
			}
			buildLogStr += runtimeLogs
		}

		// Extract detailed error information from the error message
		errStr := err.Error()

		// Look for Docker build error patterns that contain actual build output
		if strings.Contains(errStr, "failed to build") || strings.Contains(errStr, "process") {
			// Try to extract everything after "build image:" which often contains the full output
			if idx := strings.Index(errStr, "build image:"); idx != -1 {
				detailedError := errStr[idx+len("build image:"):]
				detailedError = strings.TrimSpace(detailedError)

				if detailedError != "" {
					if buildLogStr != "" {
						buildLogStr += "\n\n"
					}
					buildLogStr += detailedError
				}
			} else if strings.Contains(errStr, "process") && strings.Contains(errStr, "did not complete successfully") {
				// Extract the command that failed
				if idx := strings.Index(errStr, "process \""); idx != -1 {
					cmdStart := idx + len("process \"")
					if cmdEnd := strings.Index(errStr[cmdStart:], "\""); cmdEnd != -1 {
						failedCmd := errStr[cmdStart : cmdStart+cmdEnd]
						afterCmd := errStr[cmdStart+cmdEnd:]

						if buildLogStr != "" {
							buildLogStr += "\n"
						}
						buildLogStr += fmt.Sprintf("Build command failed: %s\n", failedCmd)

						// Look for exit code and extract surrounding context
						if exitIdx := strings.Index(afterCmd, "exit status"); exitIdx != -1 {
							remainingError := afterCmd[exitIdx:]
							lines := strings.Split(remainingError, "\n")
							for _, line := range lines {
								trimmed := strings.TrimSpace(line)
								if trimmed != "" {
									buildLogStr += trimmed + "\n"
								}
							}
						} else if strings.Contains(afterCmd, "exit code:") {
							if exitIdx := strings.Index(afterCmd, "exit code: "); exitIdx != -1 {
								exitCode := afterCmd[exitIdx+len("exit code: "):]
								if spaceIdx := strings.IndexAny(exitCode, " \n"); spaceIdx != -1 {
									exitCode = exitCode[:spaceIdx]
								}
								buildLogStr += fmt.Sprintf("Exit code: %s\n", exitCode)
							}
						}
					}
				}
			}
		}

		// If still no meaningful logs, use the full error message as fallback
		if buildLogStr == "" {
			buildLogStr = errStr
		}

		// Write comprehensive error log to file
		if s.errorLogFile != "" {
			errorLogContent := fmt.Sprintf("=== Build Error Log ===\n")
			errorLogContent += fmt.Sprintf("Container: %s\n", s.name)
			errorLogContent += fmt.Sprintf("Timestamp: %s\n", time.Now().Format(time.RFC3339))
			errorLogContent += fmt.Sprintf("Working Dir: %s\n", opts.WorkingDir)
			errorLogContent += fmt.Sprintf("Dockerfile: %s\n", s.Dockerfile)
			errorLogContent += fmt.Sprintf("\n=== Error ===\n%s\n", errStr)
			errorLogContent += fmt.Sprintf("\n=== Build Output ===\n%s\n", buildLogStr)

			if writeErr := os.WriteFile(s.errorLogFile, []byte(errorLogContent), 0644); writeErr != nil {
				fmt.Printf("Warning: failed to write error log: %v\n", writeErr)
			}
		}

		// Prepare logs for error display if verbose mode is enabled
		var logs *string
		if opts.Verbose {
			if buildLogStr != "" {
				logs = &buildLogStr
			}
		}

		return &ContainerCreationError{
			containerName: s.name,
			err:           err,
			logs:          logs,
			verbose:       opts.Verbose,
			logFilePath:   s.errorLogFile,
		}
	}

	s.sendEvent(
		opts.EventSink,
		e2eframe.EventContainerReady,
		fmt.Sprintf("HTTP unit %s started on port %d", s.Name(), s.Port),
	)

	s.cont = cont

	// Register container with cleanup registry if provided
	if opts.CleanupRegistry != nil {
		cleanableContainer := e2eframe.NewCleanableContainer(cont, s.name)
		opts.CleanupRegistry.Register(cleanableContainer)
	}

	s.endpoint = fmt.Sprintf("http://%s:%s", s.Name(), exposedPort)

	return nil
}

func (s *HTTPUnit) sendEvent(
	eventSink e2eframe.EventSink,
	eventType e2eframe.EventType,
	message string,
) {
	if eventSink != nil {
		eventSink <- &e2eframe.UnitEvent{
			BaseEvent: e2eframe.BaseEvent{
				EventType:    eventType,
				EventTime:    time.Now(),
				EventMessage: message,
			},
			UnitName: s.Name(),
			UnitKind: UnitKind,
		}
	}
}

func (s *HTTPUnit) WaitForReady(ctx context.Context) error {
	url := fmt.Sprintf("%s%s", s.ExternalEndpoint(), s.HealthcheckPath)
	client := &http.Client{Timeout: s.StartupTimeout}

	deadline, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	for {
		select {
		case <-deadline.Done():
			return fmt.Errorf("timeout waiting for http")
		default:
			resp, err := client.Get(url)
			if err == nil && resp.StatusCode == http.StatusOK {
				return nil
			}

			time.Sleep(200 * time.Millisecond)
		}
	}
}

func (s *HTTPUnit) Stop() error {
	if s.cont != nil {
		// Use Terminate() to ensure container is fully removed, not just stopped.
		// This is critical for proper network cleanup - stopped containers remain
		// attached to networks, preventing network removal and causing leaks.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err := s.cont.Terminate(ctx); err != nil {
			return fmt.Errorf("failed to terminate container: %w", err)
		}

		s.cont = nil
	}

	return nil
}

// SaveRuntimeLogs captures and saves the current container logs to a file.
// This is useful for debugging test failures where the container is running
// but the test logic fails. Returns the path to the saved log file.
func (s *HTTPUnit) SaveRuntimeLogs(suiteName, reason string) (string, error) {
	if s.cont == nil {
		return "", fmt.Errorf("container not started")
	}

	// Create log directory at project root .ene/<suite-name>/
	logDir := filepath.Join(".ene", suiteName)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create log directory: %w", err)
	}

	// Generate log file path
	timestamp := time.Now().Format("20060102-150405")
	logFilePath := filepath.Join(logDir, fmt.Sprintf("test-failure-%s-%s.log", s.name, timestamp))

	// Capture container logs
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logReader, err := s.cont.Logs(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get container logs: %w", err)
	}
	defer logReader.Close()

	logBytes, err := io.ReadAll(logReader)
	if err != nil {
		return "", fmt.Errorf("failed to read container logs: %w", err)
	}

	// Create log file content with header
	logContent := fmt.Sprintf("=== Test Failure Log ===\n")
	logContent += fmt.Sprintf("Container: %s\n", s.name)
	logContent += fmt.Sprintf("Timestamp: %s\n", time.Now().Format(time.RFC3339))
	logContent += fmt.Sprintf("Reason: %s\n", reason)
	logContent += fmt.Sprintf("\n=== Container Logs ===\n")
	logContent += string(logBytes)

	// Write to file
	if err := os.WriteFile(logFilePath, []byte(logContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write log file: %w", err)
	}

	return logFilePath, nil
}

func (s *HTTPUnit) ExternalEndpoint() string {
	return fmt.Sprintf("http://localhost:%d", s.Port)
}

func (s *HTTPUnit) LocalEndpoint() string {
	return s.endpoint
}

func (s *HTTPUnit) Get(variable string) (string, error) {
	switch variable {
	case "host":
		return "localhost", nil
	case "port":
		return fmt.Sprintf("%d", s.Port), nil
	default:
		return "", fmt.Errorf("unknown variable %s", variable)
	}
}

func (s *HTTPUnit) GetEnvRaw(opts *e2eframe.GetEnvRawOptions) map[string]string {
	envs := make(map[string]string)

	if s.EnvFile != "" {
		envFilePath := filepath.Join(opts.WorkingDir, s.EnvFile)
		if err := godotenv.Load(envFilePath); err != nil {
			return nil
		}

		envs, _ = godotenv.Read(envFilePath)
	}

	for key, value := range s.EnvVars {
		switch v := value.(type) {
		case string:
			envs[key] = v
		case int:
			envs[key] = fmt.Sprintf("%d", v)
		case bool:
			envs[key] = fmt.Sprintf("%t", v)
		default:
			return nil
		}
	}

	return envs
}

func (s *HTTPUnit) SetEnvs(env map[string]string) {
	for k, v := range env {
		s.EnvVars[k] = v
	}
}

func UnmarshallUnit(node *yaml.Node) (e2eframe.Unit, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected mapping node, got %v", node.Kind)
	}

	var config HTTPUnitConfig
	if err := node.Decode(&config); err != nil {
		return nil, fmt.Errorf("unmarshal yaml: %w", err)
	}

	httpUnit, err := New(map[string]any{
		"name":            config.Name,
		"command":         config.Command,
		"app_port":        config.AppPort,
		"dockerfile":      config.Dockerfile,
		"image":           config.Image,
		"healthcheck":     config.HealthcheckPath,
		"env_file":        config.EnvFile,
		"env":             config.Env,
		"build_timeout":   config.BuildTimeout,
		"startup_timeout": config.StartupTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("create http unit: %w", err)
	}

	return httpUnit, nil
}

// httpLogConsumer is a testcontainers.LogConsumer that captures logs from the HTTP unit.
// It implements the testcontainers.LogConsumer interface.
//
// It's not yet utilized. The goal is to record the logs to a file in case of test failures,.
type httpLogConsumer struct {
	unit *HTTPUnit
}

type containerLogCapture struct {
	buffer *strings.Builder
	mutex  *sync.Mutex
}

func (c *containerLogCapture) Accept(log testcontainers.Log) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.buffer.WriteString(string(log.Content))
}

func (c *httpLogConsumer) Accept(log testcontainers.Log) {
	c.unit.logLock.Lock()
	defer c.unit.logLock.Unlock()
	c.unit.logs = append(c.unit.logs, string(log.Content))
}

func (c *httpLogConsumer) GetLogs() []string {
	c.unit.logLock.Lock()
	defer c.unit.logLock.Unlock()

	return c.unit.logs
}

// generateSmartContentHash creates an optimized deterministic hash based on the build context
// This ensures that Docker images are rebuilt only when relevant source code changes
func (s *HTTPUnit) generateSmartContentHash(contextPath string) (string, error) {
	hash := sha256.New()

	err := filepath.Walk(contextPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and files that don't affect builds
		if info.IsDir() || s.shouldIgnoreFile(path, info) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(contextPath, path)
		if err != nil {
			return err
		}

		// Include file metadata
		hash.Write([]byte(relPath))
		hash.Write([]byte(fmt.Sprintf("%d", info.ModTime().Unix())))
		hash.Write([]byte(fmt.Sprintf("%d", info.Size())))

		// For important source files, include content
		if s.isImportantSourceFile(filepath.Ext(info.Name())) {
			if err := s.hashFileContent(hash, path, info.Size()); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate smart hash for %s: %w", contextPath, err)
	}

	result := hex.EncodeToString(hash.Sum(nil))[:12]
	return result, nil
}

// shouldIgnoreFile checks if a file should be ignored for build context hashing
func (s *HTTPUnit) shouldIgnoreFile(path string, info os.FileInfo) bool {
	name := info.Name()

	// Skip hidden files and directories (except important ones)
	if strings.HasPrefix(name, ".") && name != ".dockerignore" && name != ".env" {
		return true
	}

	// Skip common non-build files
	skipPatterns := []string{
		"**/.git/**", "**/node_modules/**", "**/vendor/**", "**/target/**",
		"**/*.log", "**/*.tmp", "**/.DS_Store", "**/tmp/**", "**/.cache/**",
		"**/coverage/**", "**/.nyc_output/**", "**/test-results/**",
		"**/dist/**", "**/build/**", "**/out/**", "**/bin/**", "**/obj/**",
	}

	for _, pattern := range skipPatterns {
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
	}

	// Skip temporary/cache file extensions
	skipSuffixes := []string{".log", ".tmp", ".cache", ".pid", ".lock", ".swp"}
	for _, suffix := range skipSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}

	return false
}

// isImportantSourceFile determines if file content should be included in hash
func (s *HTTPUnit) isImportantSourceFile(ext string) bool {
	importantExts := map[string]bool{
		".go": true, ".mod": true, ".sum": true,
		".js": true, ".ts": true, ".json": true,
		".py": true, ".rb": true, ".php": true,
		".dockerfile": true, ".Dockerfile": true,
		".yaml": true, ".yml": true, ".toml": true,
		".sql": true, ".sh": true, ".env": true,
		".html": true, ".css": true, ".scss": true,
	}

	return importantExts[ext]
}

// imageExists checks if a Docker image with the given name exists locally
func (s *HTTPUnit) imageExists(ctx context.Context, imageName string) bool {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return false
	}
	defer cli.Close()

	images, err := cli.ImageList(ctx, image.ListOptions{})
	if err != nil {
		return false
	}

	for _, img := range images {
		for _, tag := range img.RepoTags {
			if tag == imageName {
				return true
			}
		}
	}

	return false
}

// hashFileContent adds file content to hash with size optimization
func (s *HTTPUnit) hashFileContent(hash io.Writer, path string, size int64) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// For large files (>1MB), only hash first and last chunks for performance
	if size > 1024*1024 {
		// Hash first 64KB
		buffer := make([]byte, 64*1024)
		n, _ := file.Read(buffer)
		hash.Write(buffer[:n])

		// Hash last 64KB
		file.Seek(-64*1024, io.SeekEnd)
		n, _ = file.Read(buffer)
		hash.Write(buffer[:n])
	} else {
		// Hash entire file
		_, err = io.Copy(hash, file)
		if err != nil {
			return err
		}
	}

	return nil
}

// isSourceFile determines if a file extension indicates a source file
// that should have its content included in the hash
func isSourceFile(ext string) bool {
	sourceExtensions := map[string]bool{
		".go": true, ".js": true, ".ts": true, ".py": true, ".java": true,
		".c": true, ".cpp": true, ".h": true, ".hpp": true, ".rs": true,
		".rb": true, ".php": true, ".cs": true, ".swift": true, ".kt": true,
		".scala": true, ".clj": true, ".hs": true, ".ml": true, ".fs": true,
		".dockerfile": true, ".Dockerfile": true, ".yaml": true, ".yml": true,
		".json": true, ".toml": true, ".ini": true, ".conf": true, ".cfg": true,
		".sql": true, ".sh": true, ".bash": true, ".zsh": true, ".fish": true,
		".ps1": true, ".bat": true, ".cmd": true, ".html": true, ".css": true,
		".scss": true, ".sass": true, ".less": true, ".vue": true, ".jsx": true,
		".tsx": true, ".svelte": true, ".md": true, ".txt": true, ".xml": true,
	}

	return sourceExtensions[ext]
}

// stringPtr is a helper function to create a string pointer
func stringPtr(s string) *string {
	return &s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// cleanupOldCachedImages removes old ene-prefixed images to prevent cache bloat
func (s *HTTPUnit) cleanupOldCachedImages(ctx context.Context) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer cli.Close()

	// List all images with our repo prefix
	images, err := cli.ImageList(ctx, image.ListOptions{
		All: true,
	})
	if err != nil {
		return fmt.Errorf("failed to list images: %w", err)
	}

	repoPrefix := fmt.Sprintf("ene-%s", s.name)
	var imagesToRemove []image.Summary
	var totalSize int64

	// Collect images to potentially remove
	for _, img := range images {
		for _, repoTag := range img.RepoTags {
			if strings.HasPrefix(repoTag, repoPrefix+":") {
				imagesToRemove = append(imagesToRemove, img)
				totalSize += img.Size
				break
			}
		}
	}

	// Use sensible defaults for cleanup strategy
	maxCacheSize := int64(2048 * 1024 * 1024) // 2GB cache limit
	maxAge := 24 * time.Hour                  // Keep images for 24 hours

	// Sort by creation date (newest first)
	if len(imagesToRemove) > 1 {
		for i := 0; i < len(imagesToRemove)-1; i++ {
			for j := i + 1; j < len(imagesToRemove); j++ {
				if imagesToRemove[i].Created < imagesToRemove[j].Created {
					imagesToRemove[i], imagesToRemove[j] = imagesToRemove[j], imagesToRemove[i]
				}
			}
		}
	}

	// Remove images based on cache size and age
	var removedSize int64
	keepCount := 0
	currentTime := time.Now()

	for _, img := range imagesToRemove {
		imgAge := currentTime.Sub(time.Unix(img.Created, 0))

		// Keep at least 2 recent images, remove if over cache size or too old
		if keepCount >= 2 && (removedSize+img.Size > maxCacheSize || imgAge > maxAge) {
			_, err := cli.ImageRemove(ctx, img.ID, image.RemoveOptions{
				Force:         false,
				PruneChildren: true,
			})
			if err != nil {
				// Log but continue with other images
				if s.verbose {
					fmt.Printf("Warning: failed to remove image %s: %v\n", img.ID, err)
				}
			} else {
				removedSize += img.Size
				if s.verbose {
					fmt.Printf("Removed cached image %s (%.2f MB, age: %v)\n",
						img.ID[:12], float64(img.Size)/1024/1024, imgAge)
				}
			}
		} else {
			keepCount++
		}
	}

	return nil
}
