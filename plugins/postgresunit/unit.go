package postgresunit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/exapsy/ene/e2eframe"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gopkg.in/yaml.v3"
)

const (
	DefaultStartupTimeout = 30 * time.Second
	DefaultImage          = "postgres:15-alpine"
	DefaultPort           = 5432
	DefaultDatabase       = "testdb"
	DefaultUser           = "testuser"
	DefaultPassword       = "testpass"
)

type PostgresUnitConfig struct {
	Name           string        `yaml:"name"`
	Image          string        `yaml:"image"`
	Migrations     string        `yaml:"migrations"`
	AppPort        int           `yaml:"app_port"`
	EnvFile        string        `yaml:"env_file"`
	Env            any           `yaml:"env"`
	StartupTimeout time.Duration `yaml:"startup_timeout"`
	Database       string        `yaml:"database"`
	User           string        `yaml:"user"`
	Password       string        `yaml:"password"`
	Cmd            []string      `yaml:"cmd"`
}

type PostgresUnit struct {
	Network        *testcontainers.DockerNetwork
	Image          string
	MigrationsPath string
	serviceName    string
	container      testcontainers.Container
	exposedPort    int
	envFile        string
	appPort        int
	startupTimeout time.Duration
	database       string
	user           string
	password       string
	cmd            []string
	EnvVars        map[string]any
}

func init() {
	e2eframe.RegisterUnitMarshaller("postgres", UnmarshalUnit)
}

func New(cfg map[string]any) (e2eframe.Unit, error) {
	image, ok := cfg["image"].(string)
	if !ok {
		image = DefaultImage
	}

	migrations, _ := cfg["migrations"].(string)

	name := cfg["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("postgres plugin requires 'name'")
	}

	appPort, ok := cfg["app_port"].(int)
	if !ok {
		appPort = DefaultPort
	}

	envFile, ok := cfg["env_file"].(string)
	if !ok {
		envFile = ""
	}

	database, ok := cfg["database"].(string)
	if !ok {
		database = DefaultDatabase
	}

	user, ok := cfg["user"].(string)
	if !ok {
		user = DefaultUser
	}

	password, ok := cfg["password"].(string)
	if !ok {
		password = DefaultPassword
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
		envVars = make(map[string]any)
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

	return &PostgresUnit{
		Image:          image,
		MigrationsPath: migrations,
		serviceName:    name,
		appPort:        appPort,
		envFile:        envFile,
		EnvVars:        envVars,
		startupTimeout: startupTimeout,
		database:       database,
		user:           user,
		password:       password,
		cmd:            cmdArgs,
	}, nil
}

func (p *PostgresUnit) Name() string {
	return p.serviceName
}

func (p *PostgresUnit) Start(ctx context.Context, opts *e2eframe.UnitStartOptions) error {
	freePort, err := e2eframe.GetFreePort()
	if err != nil {
		return fmt.Errorf("get free port: %w", err)
	}

	p.exposedPort = freePort

	// Emit starting event
	p.sendEvent(opts.EventSink, e2eframe.EventContainerStarting,
		fmt.Sprintf("starting PostgreSQL container %s", p.serviceName))

	env := map[string]string{
		"POSTGRES_DB":       p.database,
		"POSTGRES_USER":     p.user,
		"POSTGRES_PASSWORD": p.password,
	}

	// Add any additional environment variables
	for key, value := range p.EnvVars {
		switch v := value.(type) {
		case string:
			env[key] = v
		case int:
			env[key] = fmt.Sprintf("%d", v)
		default:
			env[key] = fmt.Sprintf("%v", v)
		}
	}

	// Use custom cmd if provided, otherwise use default PostgreSQL command
	cmd := p.cmd
	if len(cmd) == 0 {
		// PostgreSQL uses default cmd from the image
		cmd = nil
	}

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Networks: []string{opts.Network.Name},
			NetworkAliases: map[string][]string{
				opts.Network.Name: {p.serviceName},
			},
			Image:        p.Image,
			Env:          env,
			ExposedPorts: []string{fmt.Sprintf("%d/tcp", DefaultPort)},
			Cmd:          cmd,
			WaitingFor: wait.ForAll(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(p.startupTimeout),
				wait.ForListeningPort("5432/tcp"),
			),
			HostConfigModifier: func(hc *container.HostConfig) {
				hc.Memory = 512 * 1024 * 1024     // 512 MB max
				hc.MemorySwap = 512 * 1024 * 1024 // no swap beyond that
			},
		},
		Started: true,
	}

	cont, err := testcontainers.GenericContainer(ctx, req)
	if err != nil {
		return fmt.Errorf("create postgres container: %w", err)
	}

	p.container = cont

	// Emit started event
	p.sendEvent(opts.EventSink, e2eframe.EventContainerStarted,
		fmt.Sprintf("PostgreSQL container %s started", p.serviceName))

	// Emit healthy event (container is healthy after WaitingFor completes)
	p.sendEvent(opts.EventSink, e2eframe.EventContainerHealthy,
		fmt.Sprintf("PostgreSQL container %s is healthy", p.serviceName))

	// Run migrations if specified
	if p.MigrationsPath != "" {
		if err := p.runMigrations(ctx, opts.WorkingDir); err != nil {
			return fmt.Errorf("run migrations: %w", err)
		}
	}

	return nil
}

func (p *PostgresUnit) WaitForReady(_ context.Context) error {
	// Container wait strategy already handles this
	return nil
}

func (p *PostgresUnit) Stop() error {
	if p.container == nil {
		return nil
	}
	return p.container.Terminate(context.Background())
}

func (p *PostgresUnit) ExternalEndpoint() string {
	if p.container == nil {
		return ""
	}

	host, err := p.container.Host(context.Background())
	if err != nil {
		return ""
	}

	port, err := p.container.MappedPort(context.Background(), "5432")
	if err != nil {
		return ""
	}

	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		p.user, p.password, host, port.Int(), p.database)
}

func (p *PostgresUnit) LocalEndpoint() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		p.user, p.password, p.serviceName, p.appPort, p.database)
}

func (p *PostgresUnit) Get(variable string) (string, error) {
	switch variable {
	case "host":
		if p.container == nil {
			return "", fmt.Errorf("postgres container not started")
		}
		host, err := p.container.Host(context.Background())
		if err != nil {
			return "", fmt.Errorf("get postgres host: %w", err)
		}
		return host, nil
	case "port":
		if p.container == nil {
			return "", fmt.Errorf("postgres container not started")
		}
		port, err := p.container.MappedPort(context.Background(), "5432")
		if err != nil {
			return "", fmt.Errorf("get postgres port: %w", err)
		}
		return port.Port(), nil
	case "database":
		return p.database, nil
	case "user":
		return p.user, nil
	case "password":
		return p.password, nil
	case "dsn", "database_url":
		return p.ExternalEndpoint(), nil
	case "local_dsn", "local_database_url":
		return p.LocalEndpoint(), nil
	default:
		return "", fmt.Errorf("variable %s not found", variable)
	}
}

func (p *PostgresUnit) runMigrations(ctx context.Context, workingDir string) error {
	migrationsPath := filepath.Join(workingDir, p.MigrationsPath)

	// Check if migrations path exists
	if _, err := os.Stat(migrationsPath); os.IsNotExist(err) {
		return fmt.Errorf("migrations path does not exist: %s", migrationsPath)
	}

	// Read all SQL files in the migrations directory
	files, err := filepath.Glob(filepath.Join(migrationsPath, "*.sql"))
	if err != nil {
		return fmt.Errorf("list migration files: %w", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no migration files found in %s", migrationsPath)
	}

	// Sort files to ensure they run in order
	for _, file := range files {
		if err := p.runSQLFile(ctx, file); err != nil {
			return fmt.Errorf("run migration %s: %w", filepath.Base(file), err)
		}
	}

	return nil
}

func (p *PostgresUnit) runSQLFile(ctx context.Context, filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read SQL file: %w", err)
	}

	// Execute the SQL content
	exitCode, output, err := p.container.Exec(ctx, []string{
		"psql",
		"-U", p.user,
		"-d", p.database,
		"-c", string(content),
	})
	if err != nil {
		return fmt.Errorf("execute SQL: %w", err)
	}

	if exitCode != 0 {
		return fmt.Errorf("SQL execution failed with exit code %d: %s", exitCode, output)
	}

	return nil
}

func (p *PostgresUnit) GetEnvRaw(opts *e2eframe.GetEnvRawOptions) map[string]string {
	envs := make(map[string]string)

	if p.envFile != "" {
		envFilePath := p.envFile
		if opts.WorkingDir != "" {
			envFilePath = filepath.Join(opts.WorkingDir, p.envFile)
		}

		file, err := os.ReadFile(envFilePath)
		if err == nil {
			lines := strings.Split(string(file), "\n")
			for _, line := range lines {
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}

				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					envs[parts[0]] = parts[1]
				}
			}
		}
	}

	for key, value := range p.EnvVars {
		switch v := value.(type) {
		case string:
			envs[key] = v
		case int:
			envs[key] = fmt.Sprintf("%d", v)
		default:
			envs[key] = fmt.Sprintf("%v", v)
		}
	}

	return envs
}

func (p *PostgresUnit) SetEnvs(env map[string]string) {
	if p.EnvVars == nil {
		p.EnvVars = make(map[string]any)
	}
	for k, v := range env {
		p.EnvVars[k] = v
	}
}

func UnmarshalUnit(node *yaml.Node) (e2eframe.Unit, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected mapping node, got %v", node.Kind)
	}

	var postgresCfg PostgresUnitConfig
	if err := node.Decode(&postgresCfg); err != nil {
		return nil, fmt.Errorf("unmarshal yaml: %w", err)
	}

	// Create a map for the New function
	cfg := map[string]any{
		"name":       postgresCfg.Name,
		"image":      postgresCfg.Image,
		"migrations": postgresCfg.Migrations,
		"app_port":   postgresCfg.AppPort,
		"env_file":   postgresCfg.EnvFile,
		"env":        postgresCfg.Env,
		"database":   postgresCfg.Database,
		"user":       postgresCfg.User,
		"password":   postgresCfg.Password,
	}

	// Convert cmd from []string to []interface{}
	if len(postgresCfg.Cmd) > 0 {
		cmd := make([]interface{}, len(postgresCfg.Cmd))
		for i, cmdItem := range postgresCfg.Cmd {
			cmd[i] = cmdItem
		}
		cfg["cmd"] = cmd
	}

	// Properly handle startup_timeout
	if postgresCfg.StartupTimeout != 0 {
		cfg["startup_timeout"] = postgresCfg.StartupTimeout
	}

	postgresUnit, err := New(cfg)
	if err != nil {
		return nil, fmt.Errorf("create postgres unit: %w", err)
	}

	return postgresUnit, nil
}

func (p *PostgresUnit) sendEvent(
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
			UnitName: p.serviceName,
			UnitKind: "postgres",
			Endpoint: fmt.Sprintf("postgres://%s:%s@%s:%d/%s", p.user, p.password, p.serviceName, p.appPort, p.database),
		}
	}
}
