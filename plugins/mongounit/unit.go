package mongounit

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"gopkg.in/yaml.v3"
	"microservice-var/cmd/e2e/e2eframe"
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

	return &MongoUnit{
		Image:             image,
		MigrationFilePath: migrations,
		serviceName:       name,
		appPort:           appPort,
		envFile:           envFile,
		EnvVars:           envVars,
		startupTimeout:    startupTimeout,
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

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Networks: []string{opts.Network.Name},
			NetworkAliases: map[string][]string{
				opts.Network.Name: {m.serviceName},
			},
			//ExposedPorts: []string{fmt.Sprintf("%d/tcp", freePort)},
			Image: "mongo:6.0",
			HostConfigModifier: func(hc *container.HostConfig) {
				hc.Memory = 512 * 1024 * 1024     // 512 MB max
				hc.MemorySwap = 512 * 1024 * 1024 // no swap beyond that
			},
			Cmd: []string{
				"mongod", "--bind_ip_all",
				"--wiredTigerCacheSizeGB", "0.25",
			},
		},
		Started: true,
	}

	cont, err := testcontainers.GenericContainer(ctx, req)
	if err != nil {
		return fmt.Errorf("create mongo container: %w", err)
	}

	m.container = cont

	return nil
}

func (m *MongoUnit) WaitForReady(_ context.Context) error {
	// Container wait strategy already handles this
	return nil
}

func (m *MongoUnit) Stop() error {
	return m.container.Terminate(context.Background())
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
	//host, _ := m.container.Host(context.Background())
	//port, _ := m.container.MappedPort(context.Background(), "27017")
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

func (m *MongoUnit) migrate(ctx context.Context, _ string) error {
	host, err := m.container.Host(ctx)
	if err != nil {
		return fmt.Errorf("get mongo host: %w", err)
	}

	port, err := m.container.MappedPort(ctx, "27017")
	if err != nil {
		return fmt.Errorf("get mongo port: %w", err)
	}

	migrationContent, err := os.ReadFile(m.MigrationFilePath)
	if err != nil {
		return fmt.Errorf("read migration file: %w", err)
	}

	// Execute the migration script
	_, _, err = m.container.Exec(ctx, []string{
		"mongosh",
		"--host", host,
		"--port", port.Port(),
		"--eval", string(migrationContent),
	})
	if err != nil {
		return fmt.Errorf("execute migration: %w", err)
	}

	return nil
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

func (m *MongoUnit) GetEnvRaw() map[string]string {
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
	for k, v := range env {
		s.EnvVars[k] = v
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
