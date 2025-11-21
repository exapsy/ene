package mongounit

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/exapsy/ene/e2eframe"
	"github.com/testcontainers/testcontainers-go"
	"gopkg.in/yaml.v3"
)

const (
	DefaultStartupTimeout = 5 * time.Second
)

type MongoUnitConfig struct {
	Name           string        `yaml:"name"`
	Image          string        `yaml:"image"`
	MigrationFile  string        `yaml:"migration_file"`
	AppPort        int           `yaml:"app_port"`
	EnvFile        string        `yaml:"env_file"`
	Env            any           `yaml:"env"`
	StartupTimeout time.Duration `yaml:"startup_timeout"`
	Cmd            []string      `yaml:"cmd"`
}

type MongoUnit struct {
	Network           *testcontainers.DockerNetwork
	Image             string
	MigrationFilePath string
	serviceName       string
	container         testcontainers.Container
	dsn               string
	exposedPort       int
	envFile           string
	appPort           int
	startupTimeout    time.Duration
	cmd               []string
	EnvVars           map[string]any
}

func init() {
	e2eframe.RegisterUnitMarshaller("mongo", UnmarshalUnit)
}

func New(cfg map[string]any) (e2eframe.Unit, error) {
	image, ok := cfg["image"].(string)
	if !ok {
		image = "mongo:6"
	}

	migrations, _ := cfg["migrations"].(string)

	name := cfg["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("mongo plugin requires 'name'")
	}

	appPort, ok := cfg["app_port"].(int)
	if !ok {
		appPort = 27017
	}

	envFile, ok := cfg["env_file"].(string)
	if !ok {
		envFile = ""
	}
	// Handle startup timeout more robustly
	var startupTimeout time.Duration
	switch t := cfg["startup_timeout"].(type) {
	case time.Duration:
		startupTimeout = t
	case string:
		var err error

		startupTimeout, err = time.ParseDuration(t)
		if err != nil {
			startupTimeout = DefaultStartupTimeout
		}
	default:
		startupTimeout = DefaultStartupTimeout
	}

	envVars, ok := cfg["env"].(map[string]any)
	if !ok {
		envVars = nil
	}

	// Handle cmd
	var cmdArgs []string
	if cmdRaw, ok := cfg["cmd"].([]interface{}); ok {
		for _, cmdItem := range cmdRaw {
			if cmdStr, ok := cmdItem.(string); ok {
				cmdArgs = append(cmdArgs, cmdStr)
			}
		}
	}

	return &MongoUnit{
		Image:             image,
		MigrationFilePath: migrations,
		serviceName:       name,
		appPort:           appPort,
		envFile:           envFile,
		EnvVars:           envVars,
		startupTimeout:    startupTimeout,
		cmd:               cmdArgs,
	}, nil
}

func (m *MongoUnit) Name() string {
	return m.serviceName
}

func (m *MongoUnit) Start(ctx context.Context, opts *e2eframe.UnitStartOptions) error {
	freePort, err := e2eframe.GetFreePort()
	if err != nil {
		return fmt.Errorf("get free port: %w", err)
	}

	m.exposedPort = freePort

	// Use custom cmd if provided, otherwise use default MongoDB command
	cmd := m.cmd
	if len(cmd) == 0 {
		cmd = []string{
			"mongod", "--bind_ip_all",
			"--wiredTigerCacheSizeGB", "0.25",
		}
	}

	// Emit starting event
	m.sendEvent(opts.EventSink, e2eframe.EventContainerStarting,
		fmt.Sprintf("starting MongoDB container %s", m.serviceName))

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Networks: []string{opts.Network.Name},
			NetworkAliases: map[string][]string{
				opts.Network.Name: {m.serviceName},
			},
			// ExposedPorts: []string{fmt.Sprintf("%d/tcp", freePort)},
			Image: m.Image,
			HostConfigModifier: func(hc *container.HostConfig) {
				hc.Memory = 512 * 1024 * 1024     // 512 MB max
				hc.MemorySwap = 512 * 1024 * 1024 // no swap beyond that
			},
			Cmd: cmd,
		},
		Started: true,
	}

	cont, err := testcontainers.GenericContainer(ctx, req)
	if err != nil {
		return fmt.Errorf("create mongo container: %w", err)
	}

	m.container = cont

	// Register container with cleanup registry if provided
	if opts.CleanupRegistry != nil {
		cleanableContainer := e2eframe.NewCleanableContainer(cont, m.serviceName)
		opts.CleanupRegistry.Register(cleanableContainer)
	}

	// Wait for MongoDB to be ready before running migrations
	if err := m.waitForMongoDB(ctx); err != nil {
		return fmt.Errorf("wait for mongodb: %w", err)
	}

	// Emit ready event (container is ready after health check completes)
	m.sendEvent(opts.EventSink, e2eframe.EventContainerReady,
		fmt.Sprintf("MongoDB container %s is ready", m.serviceName))

	// Run migrations if specified
	if m.MigrationFilePath != "" {
		if err := m.migrate(ctx, opts.WorkingDir); err != nil {
			return fmt.Errorf("run migrations: %w", err)
		}
	}

	return nil
}

func (m *MongoUnit) migrate(ctx context.Context, workingDir string) error {
	// Resolve migration file path relative to working directory
	migrationPath := filepath.Join(workingDir, m.MigrationFilePath)

	// Check if migration file exists
	if _, err := os.Stat(migrationPath); os.IsNotExist(err) {
		return fmt.Errorf("migration file does not exist: %s", migrationPath)
	}

	fmt.Printf("📦 Running MongoDB migrations from '%s'...\n", m.MigrationFilePath)

	migrationContent, err := os.ReadFile(migrationPath)
	if err != nil {
		return fmt.Errorf("read migration file %s: %w", migrationPath, err)
	}

	// Execute the migration script using mongosh
	// We use --quiet to suppress unnecessary output and --eval to execute the script content
	exitCode, output, err := m.container.Exec(ctx, []string{
		"mongosh",
		"--quiet",
		"--eval", string(migrationContent),
	})

	if err != nil {
		return fmt.Errorf("execute migration script: %w", err)
	}

	// Read the output
	outputBytes, readErr := io.ReadAll(output)
	if readErr != nil {
		return fmt.Errorf("read migration output: %w", readErr)
	}

	if exitCode != 0 {
		fmt.Printf("❌ Migration failed\n")
		return fmt.Errorf("migration script failed with exit code %d: %s", exitCode, string(outputBytes))
	}

	// Print migration output if there is any
	outputStr := strings.TrimSpace(string(outputBytes))
	if outputStr != "" {
		fmt.Println(outputStr)
	}

	fmt.Printf("✅ MongoDB migrations completed successfully for unit '%s'\n", m.serviceName)

	return nil
}

func (m *MongoUnit) waitForMongoDB(ctx context.Context) error {
	// Wait for MongoDB to be ready by attempting to connect
	maxRetries := 30
	retryInterval := time.Second

	for i := 0; i < maxRetries; i++ {
		// Try to execute a simple command to check if MongoDB is ready
		exitCode, _, err := m.container.Exec(ctx, []string{
			"mongosh",
			"--quiet",
			"--eval", "db.adminCommand('ping')",
		})

		if err == nil && exitCode == 0 {
			return nil
		}

		if i < maxRetries-1 {
			time.Sleep(retryInterval)
		}
	}

	return fmt.Errorf("mongodb did not become ready within %d seconds", maxRetries)
}

func (m *MongoUnit) WaitForReady(_ context.Context) error {
	// Container wait strategy already handles this
	return nil
}

func (m *MongoUnit) Stop() error {
	if m.container == nil {
		return nil
	}
	return m.container.Terminate(context.Background())
}

// SaveRuntimeLogs captures and saves the current container logs to a file.
// This is useful for debugging test failures where the container is running
// but the test logic fails. Returns the path to the saved log file.
func (m *MongoUnit) SaveRuntimeLogs(suiteName, reason string) (string, error) {
	if m.container == nil {
		return "", fmt.Errorf("container not started")
	}

	// Create log directory at project root .ene/<suite-name>/
	logDir := filepath.Join(".ene", suiteName)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create log directory: %w", err)
	}

	// Generate log file path
	timestamp := time.Now().Format("20060102-150405")
	logFilePath := filepath.Join(logDir, fmt.Sprintf("test-failure-%s-%s.log", m.serviceName, timestamp))

	// Capture container logs
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logReader, err := m.container.Logs(ctx)
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
	logContent += fmt.Sprintf("Container: %s\n", m.serviceName)
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

func (m *MongoUnit) ExternalEndpoint() string {
	host, err := m.container.Host(context.Background())
	if err != nil {
		return ""
	}

	port, err := m.container.MappedPort(context.Background(), "27017")
	if err != nil {
		return ""
	}

	return fmt.Sprintf("mongodb://%s:%d", host, port.Int())
}

func (m *MongoUnit) LocalEndpoint() string {
	// host, _ := m.container.Host(context.Background())
	// port, _ := m.container.MappedPort(context.Background(), "27017")
	return fmt.Sprintf("mongodb://%s:%d", m.serviceName, m.appPort)
}

func (m *MongoUnit) Get(variable string) (string, error) {
	switch variable {
	case "host":
		if m.container == nil {
			return "", fmt.Errorf("mongo container not started")
		}

		host, err := m.container.Host(context.Background())
		if err != nil {
			return "", fmt.Errorf("get mongo host: %w", err)
		}

		return host, nil
	case "port":
		if m.container == nil {
			return "", fmt.Errorf("mongo container not started")
		}

		exposedPort := fmt.Sprintf("%d", m.exposedPort)

		return exposedPort, nil
	case "dsn":
		if m.container == nil {
			return "", fmt.Errorf("mongo container not started")
		}

		host := m.serviceName

		return fmt.Sprintf("mongodb://%s:%d/test", host, m.appPort), nil
	}

	return "", fmt.Errorf("variable %s not found", variable)
}

func (m *MongoUnit) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("expected mapping node, got %v", node.Kind)
	}

	var cfg map[string]any
	if err := node.Decode(&cfg); err != nil {
		return fmt.Errorf("decode yaml: %w", err)
	}

	image, ok := cfg["image"].(string)
	if !ok {
		image = "mongo:6"
	}

	m.Image = image
	migrations, _ := cfg["migrations"].(string)
	m.MigrationFilePath = migrations
	name, _ := cfg["name"].(string)
	m.serviceName = name

	return nil
}

func (m *MongoUnit) GetEnvRaw(_ *e2eframe.GetEnvRawOptions) map[string]string {
	envs := make(map[string]string)

	if m.envFile != "" {
		file, err := os.ReadFile(m.envFile)
		if err != nil {
			return nil
		}

		lines := strings.Split(string(file), "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}

			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				envs[parts[0]] = parts[1]
			}
		}
	}

	for key, value := range m.EnvVars {
		switch v := value.(type) {
		case string:
			envs[key] = v
		case int:
			envs[key] = fmt.Sprintf("%d", v)
		default:
			return nil
		}
	}

	return envs
}

func (s *MongoUnit) SetEnvs(env map[string]string) {
	for key, val := range env {
		s.EnvVars[key] = val
	}
}

func (m *MongoUnit) sendEvent(
	eventSink e2eframe.EventSink,
	eventType e2eframe.EventType,
	message string,
) {
	if eventSink != nil {
		// Construct endpoint dynamically
		endpoint := fmt.Sprintf("mongodb://%s:%d", m.serviceName, m.appPort)

		eventSink <- &e2eframe.UnitEvent{
			BaseEvent: e2eframe.BaseEvent{
				EventType:    eventType,
				EventTime:    time.Now(),
				EventMessage: message,
			},
			UnitName: m.serviceName,
			UnitKind: "mongo",
			Endpoint: endpoint,
		}
	}
}

func UnmarshalUnit(node *yaml.Node) (e2eframe.Unit, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected mapping node, got %v", node.Kind)
	}

	var mongoCfg MongoUnitConfig
	if err := node.Decode(&mongoCfg); err != nil {
		return nil, fmt.Errorf("unmarshal yaml: %w", err)
	}

	// Create a map for the New function
	cfg := map[string]any{
		"name":       mongoCfg.Name,
		"image":      mongoCfg.Image,
		"migrations": mongoCfg.MigrationFile,
		"app_port":   mongoCfg.AppPort,
		"env_file":   mongoCfg.EnvFile,
		"env":        mongoCfg.Env,
	}

	// Convert cmd from []string to []interface{}
	if len(mongoCfg.Cmd) > 0 {
		cmd := make([]interface{}, len(mongoCfg.Cmd))
		for i, cmdItem := range mongoCfg.Cmd {
			cmd[i] = cmdItem
		}
		cfg["cmd"] = cmd
	}

	// Properly handle startup_timeout
	if mongoCfg.StartupTimeout != 0 {
		cfg["startup_timeout"] = mongoCfg.StartupTimeout
	}

	mongoUnit, err := New(cfg)
	if err != nil {
		return nil, fmt.Errorf("create mongo unit: %w", err)
	}

	return mongoUnit, nil
}
