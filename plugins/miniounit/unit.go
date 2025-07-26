package miniounit

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/exapsy/ene/e2eframe"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/testcontainers/testcontainers-go"
	"gopkg.in/yaml.v3"
)

const (
	DefaultStartupTimeout = 10 * time.Second
	DefaultAccessKey      = "minioadmin"
	DefaultSecretKey      = "minioadmin"
	DefaultPort           = 9000
	DefaultConsolePort    = 9001
)

type MinioUnitConfig struct {
	Name           string        `yaml:"name"`
	Image          string        `yaml:"image"`
	AccessKey      string        `yaml:"access_key"`
	SecretKey      string        `yaml:"secret_key"`
	AppPort        int           `yaml:"app_port"`
	ConsolePort    int           `yaml:"console_port"`
	EnvFile        string        `yaml:"env_file"`
	Env            any           `yaml:"env"`
	StartupTimeout time.Duration `yaml:"startup_timeout"`
	Buckets        []string      `yaml:"buckets"`
	Cmd            []string      `yaml:"cmd"`
}

type MinioUnit struct {
	Network        *testcontainers.DockerNetwork
	Image          string
	serviceName    string
	container      testcontainers.Container
	exposedPort    int
	consolePort    int
	envFile        string
	appPort        int
	startupTimeout time.Duration
	accessKey      string
	secretKey      string
	buckets        []string
	cmd            []string
	EnvVars        map[string]any
}

func init() {
	e2eframe.RegisterUnitMarshaller("minio", UnmarshalUnit)
}

func New(cfg map[string]any) (e2eframe.Unit, error) {
	image, ok := cfg["image"].(string)
	if !ok {
		image = "minio/minio:latest"
	}

	name, ok := cfg["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("minio plugin requires 'name'")
	}

	appPort, ok := cfg["app_port"].(int)
	if !ok {
		appPort = DefaultPort
	}

	consolePort, ok := cfg["console_port"].(int)
	if !ok {
		consolePort = DefaultConsolePort
	}

	accessKey, ok := cfg["access_key"].(string)
	if !ok {
		accessKey = DefaultAccessKey
	}

	secretKey, ok := cfg["secret_key"].(string)
	if !ok {
		secretKey = DefaultSecretKey
	}

	// Handle command configuration
	var cmd []string
	if cmdIface, ok := cfg["command"].(any); ok {
		switch cmdIface := cmdIface.(type) {
		case []any:
			cmd = make([]string, len(cmdIface))
			for i, v := range cmdIface {
				switch v := v.(type) {
				case string:
					cmd[i] = v
				default:
					return nil, fmt.Errorf("minio plugin requires 'command' to be a list of strings")
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
			return nil, fmt.Errorf("minio plugin requires 'command' to be a list of strings")
		}
	} else {
		// Default Minio command
		cmd = []string{
			"server", "/data",
			"--console-address", fmt.Sprintf(":%d", consolePort),
		}
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
		envVars = make(map[string]any)
	}

	// Handle buckets
	var buckets []string
	if bucketsRaw, ok := cfg["buckets"].([]interface{}); ok {
		for _, bucket := range bucketsRaw {
			if bucketStr, ok := bucket.(string); ok {
				buckets = append(buckets, bucketStr)
			}
		}
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

	return &MinioUnit{
		Image:          image,
		serviceName:    name,
		appPort:        appPort,
		consolePort:    consolePort,
		envFile:        envFile,
		EnvVars:        envVars,
		startupTimeout: startupTimeout,
		accessKey:      accessKey,
		secretKey:      secretKey,
		buckets:        buckets,
		cmd:            cmdArgs,
	}, nil
}

func (m *MinioUnit) Name() string {
	return m.serviceName
}

func (m *MinioUnit) Start(ctx context.Context, opts *e2eframe.UnitStartOptions) error {
	freePort, err := e2eframe.GetFreePort()
	if err != nil {
		return fmt.Errorf("get free port: %w", err)
	}

	consolePort, err := e2eframe.GetFreePort()
	if err != nil {
		return fmt.Errorf("get free console port: %w", err)
	}

	m.exposedPort = freePort
	m.consolePort = consolePort

	// Set up environment variables
	env := map[string]string{
		"MINIO_ROOT_USER":     m.accessKey,
		"MINIO_ROOT_PASSWORD": m.secretKey,
	}

	// Add custom environment variables
	for key, value := range m.EnvVars {
		switch v := value.(type) {
		case string:
			env[key] = v
		case int:
			env[key] = fmt.Sprintf("%d", v)
		default:
			env[key] = fmt.Sprintf("%v", v)
		}
	}

	// Use custom cmd if provided, otherwise use default Minio command
	cmd := m.cmd
	if len(cmd) == 0 {
		cmd = []string{
			"server", "/data",
			"--console-address", fmt.Sprintf(":%d", m.consolePort),
		}
	}

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Networks: []string{opts.Network.Name},
			NetworkAliases: map[string][]string{
				opts.Network.Name: {m.serviceName},
			},
			Image: m.Image,
			Env:   env,
			ExposedPorts: []string{
				fmt.Sprintf("%d/tcp", m.appPort),
				fmt.Sprintf("%d/tcp", m.consolePort),
			},
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
		return fmt.Errorf("create minio container: %w", err)
	}

	m.container = cont

	// Wait for Minio to be ready and create buckets if specified
	if err := m.createBuckets(ctx); err != nil {
		return fmt.Errorf("create buckets: %w", err)
	}

	return nil
}

func (m *MinioUnit) WaitForReady(ctx context.Context) error {
	if m.container == nil {
		return fmt.Errorf("container not started")
	}

	// Test connection to Minio
	host, err := m.container.Host(ctx)
	if err != nil {
		return fmt.Errorf("get minio host: %w", err)
	}

	port, err := m.container.MappedPort(ctx, "9000")
	if err != nil {
		return fmt.Errorf("get minio port: %w", err)
	}

	endpoint := fmt.Sprintf("%s:%d", host, port.Int())
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(m.accessKey, m.secretKey, ""),
		Secure: false,
	})
	if err != nil {
		return fmt.Errorf("create minio client: %w", err)
	}

	// Try to list buckets to verify connection
	_, err = client.ListBuckets(ctx)
	if err != nil {
		return fmt.Errorf("test minio connection: %w", err)
	}

	return nil
}

func (m *MinioUnit) Stop() error {
	if m.container != nil {
		return m.container.Terminate(context.Background())
	}
	return nil
}

func (m *MinioUnit) ExternalEndpoint() string {
	if m.container == nil {
		return ""
	}

	host, err := m.container.Host(context.Background())
	if err != nil {
		return ""
	}

	port, err := m.container.MappedPort(context.Background(), "9000")
	if err != nil {
		return ""
	}

	return fmt.Sprintf("%s:%d", host, port.Int())
}

func (m *MinioUnit) LocalEndpoint() string {
	return fmt.Sprintf("%s:%d", m.serviceName, m.appPort)
}

func (m *MinioUnit) Get(variable string) (string, error) {
	switch variable {
	case "host":
		if m.container == nil {
			return "", fmt.Errorf("minio container not started")
		}

		host, err := m.container.Host(context.Background())
		if err != nil {
			return "", fmt.Errorf("get minio host: %w", err)
		}

		return host, nil
	case "port":
		if m.container == nil {
			return "", fmt.Errorf("minio container not started")
		}

		port, err := m.container.MappedPort(context.Background(), "9000")
		if err != nil {
			return "", fmt.Errorf("get minio port: %w", err)
		}

		return port.Port(), nil
	case "endpoint":
		return m.ExternalEndpoint(), nil
	case "local_endpoint":
		return m.LocalEndpoint(), nil
	case "access_key":
		return m.accessKey, nil
	case "secret_key":
		return m.secretKey, nil
	case "console_port":
		if m.container == nil {
			return "", fmt.Errorf("minio container not started")
		}

		port, err := m.container.MappedPort(context.Background(), "9001")
		if err != nil {
			return "", fmt.Errorf("get minio console port: %w", err)
		}

		return port.Port(), nil
	case "console_endpoint":
		if m.container == nil {
			return "", fmt.Errorf("minio container not started")
		}

		host, err := m.container.Host(context.Background())
		if err != nil {
			return "", fmt.Errorf("get minio host: %w", err)
		}

		port, err := m.container.MappedPort(context.Background(), "9001")
		if err != nil {
			return "", fmt.Errorf("get minio console port: %w", err)
		}

		return fmt.Sprintf("%s:%d", host, port.Int()), nil
	}

	return "", fmt.Errorf("variable %s not found", variable)
}

func (m *MinioUnit) createBuckets(ctx context.Context) error {
	if len(m.buckets) == 0 {
		return nil
	}

	// Wait a moment for Minio to be fully ready
	time.Sleep(2 * time.Second)

	endpoint := m.ExternalEndpoint()
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(m.accessKey, m.secretKey, ""),
		Secure: false,
	})
	if err != nil {
		return fmt.Errorf("create minio client: %w", err)
	}

	for _, bucketName := range m.buckets {
		exists, err := client.BucketExists(ctx, bucketName)
		if err != nil {
			return fmt.Errorf("check bucket %s existence: %w", bucketName, err)
		}

		if !exists {
			err = client.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
			if err != nil {
				return fmt.Errorf("create bucket %s: %w", bucketName, err)
			}
		}
	}

	return nil
}

func (m *MinioUnit) GetEnvRaw(_ *e2eframe.GetEnvRawOptions) map[string]string {
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
			envs[key] = fmt.Sprintf("%v", v)
		}
	}

	// Add Minio-specific environment variables
	envs["MINIO_ROOT_USER"] = m.accessKey
	envs["MINIO_ROOT_PASSWORD"] = m.secretKey
	envs["MINIO_ENDPOINT"] = m.LocalEndpoint()

	return envs
}

func (m *MinioUnit) SetEnvs(env map[string]string) {
	if m.EnvVars == nil {
		m.EnvVars = make(map[string]any)
	}
	for k, v := range env {
		m.EnvVars[k] = v
	}
}

func UnmarshalUnit(node *yaml.Node) (e2eframe.Unit, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected mapping node, got %v", node.Kind)
	}

	var minioCfg MinioUnitConfig
	if err := node.Decode(&minioCfg); err != nil {
		return nil, fmt.Errorf("unmarshal yaml: %w", err)
	}

	// Create a map for the New function
	cfg := map[string]any{
		"name":         minioCfg.Name,
		"image":        minioCfg.Image,
		"access_key":   minioCfg.AccessKey,
		"secret_key":   minioCfg.SecretKey,
		"app_port":     minioCfg.AppPort,
		"console_port": minioCfg.ConsolePort,
		"env_file":     minioCfg.EnvFile,
		"env":          minioCfg.Env,
	}

	// Convert buckets from []string to []interface{}
	if len(minioCfg.Buckets) > 0 {
		buckets := make([]interface{}, len(minioCfg.Buckets))
		for i, bucket := range minioCfg.Buckets {
			buckets[i] = bucket
		}
		cfg["buckets"] = buckets
	}

	// Convert cmd from []string to []interface{}
	if len(minioCfg.Cmd) > 0 {
		cmd := make([]interface{}, len(minioCfg.Cmd))
		for i, cmdItem := range minioCfg.Cmd {
			cmd[i] = cmdItem
		}
		cfg["cmd"] = cmd
	}

	// Properly handle startup_timeout
	if minioCfg.StartupTimeout != 0 {
		cfg["startup_timeout"] = minioCfg.StartupTimeout
	}

	minioUnit, err := New(cfg)
	if err != nil {
		return nil, fmt.Errorf("create minio unit: %w", err)
	}

	return minioUnit, nil
}
