package httpplugin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/exapsy/ene/e2eframe"
	"github.com/joho/godotenv"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gopkg.in/yaml.v3"
)

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

	// For capturing logs
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
			return fmt.Errorf("could not load env file: %w", err)
		}

		envs, err = godotenv.Read(envFile)
		if err != nil {
			return fmt.Errorf("could not read env file: %w", err)
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

	dockerfileDir := filepath.Join(opts.WorkingDir)
	dockerfileDir, err = filepath.Abs(dockerfileDir)
	if err != nil {
		return fmt.Errorf("get absolute path of dockerfile directory: %w", err)
	}

	cwd, err := os.Getwd()
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

	var fromDockerfile testcontainers.FromDockerfile
	if s.Dockerfile != "" {
		fromDockerfile = testcontainers.FromDockerfile{
			Context:        dockerBaseDir,
			Dockerfile:     s.Dockerfile,
			BuildLogWriter: buildLogWriter,
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
		return fmt.Errorf("get free port: %w", err)
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
			hostConfig.Memory = 2 * 512 * 1024 * 1024     // 512 MB max
			hostConfig.MemorySwap = 2 * 512 * 1024 * 1024 // no swap beyond that
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
			WithStartupTimeout(s.StartupTimeout),
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
		return fmt.Errorf("create container: %w", err)
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
		return s.cont.Terminate(context.Background())
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
