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
	"github.com/exapsy/ene/e2eframe"
	"github.com/joho/godotenv"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gopkg.in/yaml.v3"
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

type ContainerCreationError struct {
	containerName string
	err           error
}

func (e *ContainerCreationError) Error() string {
	return fmt.Sprintf("failed to create container %s: %v", e.containerName, e.err)
}

func (e *ContainerCreationError) UserFriendlyMessage() string {
	errMsg := e.err.Error()
	prefix := fmt.Sprintf("%s build failed", e.containerName)

	// Provide helpful hints for common errors
	if strings.Contains(errMsg, "not found") && strings.Contains(errMsg, ".netrc") {
		return fmt.Sprintf("%s: .netrc file not found (required for private Go modules)", prefix)
	}
	if strings.Contains(errMsg, "not found") && (strings.Contains(errMsg, "go.mod") || strings.Contains(errMsg, "go.sum")) {
		return fmt.Sprintf("%s: go.mod or go.sum not found (check Dockerfile context)", prefix)
	}
	if strings.Contains(errMsg, "exit code: 1") && strings.Contains(errMsg, "go mod download") {
		return fmt.Sprintf("%s: go mod download failed (check .netrc credentials or network)", prefix)
	}
	if strings.Contains(errMsg, "Dockerfile") && strings.Contains(errMsg, "not found") {
		return fmt.Sprintf("%s: Dockerfile not found", prefix)
	}

	// For other errors, show a truncated version of the actual error
	if len(errMsg) > 120 {
		return fmt.Sprintf("%s: %s...", prefix, errMsg[:117])
	}
	return fmt.Sprintf("%s: %s", prefix, errMsg)
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
	DefaultStartupTimeout = 5 * time.Second
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
	StartupTimeout  time.Duration
	endpoint        string
	verbose         bool

	// Performance optimizations
	buildSemaphore chan struct{}

	// logging
	logs    []string
	logLock sync.Mutex
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

	if opts.Debug {
		buildLogWriter = os.Stdout
		logConsumers = []testcontainers.LogConsumer{
			&testcontainers.StdoutLogConsumer{},
			logCapture,
		}
	} else {
		buildLogWriter = io.Discard
		logConsumers = []testcontainers.LogConsumer{logCapture}
	}

	// Cleanup old cached images if requested
	if opts.CleanupCache {
		if err := s.cleanupOldCachedImages(ctx); err != nil {
			// Log warning but don't fail the test
			fmt.Printf("Warning: failed to cleanup old cached images: %v\n", err)
		}
	}

	var fromDockerfile testcontainers.FromDockerfile
	if s.Dockerfile != "" {
		// Acquire build semaphore to limit concurrent builds
		s.buildSemaphore <- struct{}{}
		defer func() { <-s.buildSemaphore }()

		// Generate smart content-based tag for better cache behavior
		contentHash, err := s.generateSmartContentHash(dockerBaseDir)
		if err != nil {
			return fmt.Errorf("failed to generate smart content hash: %w", err)
		}

		fromDockerfile = testcontainers.FromDockerfile{
			Context:        dockerBaseDir,
			Dockerfile:     filepath.Base(dockerfilePath),
			BuildLogWriter: buildLogWriter,
			Repo:           fmt.Sprintf("ene-%s", s.name),
			Tag:            contentHash,
			BuildOptionsModifier: func(buildOptions *types.ImageBuildOptions) {
				// Enable BuildKit for better performance
				buildOptions.Version = types.BuilderBuildKit
				buildOptions.BuildArgs = map[string]*string{
					"BUILDKIT_PROGRESS": stringPtr("plain"),
					"DOCKER_BUILDKIT":   stringPtr("1"),
				}

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

	var image string
	if s.Image != "" {
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

	cont, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return &ContainerCreationError{containerName: s.name, err: err}
	}

	s.sendEvent(
		opts.EventSink,
		e2eframe.EventContainerStarted,
		fmt.Sprintf("HTTP unit %s started on port %d", s.Name(), s.Port),
	)

	s.cont = cont

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
		// First try to stop gracefully with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Try graceful stop first
		if err := s.cont.Stop(ctx, nil); err != nil {
			// If graceful stop fails, force terminate
			terminateCtx, terminateCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer terminateCancel()

			if termErr := s.cont.Terminate(terminateCtx); termErr != nil {
				return fmt.Errorf("failed to stop container gracefully (%v) and terminate (%v)", err, termErr)
			}
		}

		s.cont = nil
	}

	return nil
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
