package miniounit

import (
	"context"
	"testing"
	"time"

	"github.com/exapsy/ene/e2eframe"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"gopkg.in/yaml.v3"
)

func TestMinioUnit_New(t *testing.T) {
	tests := []struct {
		name    string
		cfg     map[string]any
		wantErr bool
	}{
		{
			name: "valid config with defaults",
			cfg: map[string]any{
				"name": "test-minio",
			},
			wantErr: false,
		},
		{
			name: "valid config with custom values",
			cfg: map[string]any{
				"name":         "test-minio",
				"image":        "minio/minio:RELEASE.2024-01-16T16-07-38Z",
				"access_key":   "testuser",
				"secret_key":   "testpass123",
				"app_port":     9000,
				"console_port": 9001,
				"buckets":      []interface{}{"bucket1", "bucket2"},
			},
			wantErr: false,
		},
		{
			name: "valid config with custom cmd",
			cfg: map[string]any{
				"name":       "test-minio",
				"access_key": "testuser",
				"secret_key": "testpass123",
				"cmd":        []interface{}{"server", "/custom-data", "--console-address", ":9090"},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			cfg: map[string]any{
				"image": "minio/minio:latest",
			},
			wantErr: true,
		},
		{
			name: "with startup timeout as string",
			cfg: map[string]any{
				"name":            "test-minio",
				"startup_timeout": "30s",
			},
			wantErr: false,
		},
		{
			name: "with startup timeout as duration",
			cfg: map[string]any{
				"name":            "test-minio",
				"startup_timeout": 30 * time.Second,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unit, err := New(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, unit)

			minioUnit, ok := unit.(*MinioUnit)
			require.True(t, ok)

			if name, exists := tt.cfg["name"]; exists {
				assert.Equal(t, name.(string), minioUnit.Name())
			}

			if image, exists := tt.cfg["image"]; exists {
				assert.Equal(t, image.(string), minioUnit.Image)
			} else {
				assert.Equal(t, "minio/minio:latest", minioUnit.Image)
			}

			if accessKey, exists := tt.cfg["access_key"]; exists {
				assert.Equal(t, accessKey.(string), minioUnit.accessKey)
			} else {
				assert.Equal(t, DefaultAccessKey, minioUnit.accessKey)
			}

			if secretKey, exists := tt.cfg["secret_key"]; exists {
				assert.Equal(t, secretKey.(string), minioUnit.secretKey)
			} else {
				assert.Equal(t, DefaultSecretKey, minioUnit.secretKey)
			}
		})
	}
}

func TestMinioUnit_UnmarshalYAML(t *testing.T) {
	yamlContent := `
name: test-minio
image: minio/minio:latest
access_key: testuser
secret_key: testpass123
app_port: 9000
console_port: 9001
startup_timeout: 30s
buckets:
  - bucket1
  - bucket2
cmd:
  - server
  - /custom-data
  - --console-address
  - ":9090"
env:
  CUSTOM_VAR: "custom_value"
`

	var node yaml.Node
	err := yaml.Unmarshal([]byte(yamlContent), &node)
	require.NoError(t, err)

	// Get the document node (first child of root)
	require.Len(t, node.Content, 1)
	unit, err := UnmarshalUnit(node.Content[0])
	require.NoError(t, err)
	assert.NotNil(t, unit)

	minioUnit, ok := unit.(*MinioUnit)
	require.True(t, ok)

	assert.Equal(t, "test-minio", minioUnit.Name())
	assert.Equal(t, "minio/minio:latest", minioUnit.Image)
	assert.Equal(t, "testuser", minioUnit.accessKey)
	assert.Equal(t, "testpass123", minioUnit.secretKey)
	assert.Equal(t, 9000, minioUnit.appPort)
	assert.Equal(t, 9001, minioUnit.consolePort)
	assert.Equal(t, []string{"bucket1", "bucket2"}, minioUnit.buckets)
	assert.Equal(t, []string{"server", "/custom-data", "--console-address", ":9090"}, minioUnit.cmd)
	assert.Contains(t, minioUnit.EnvVars, "CUSTOM_VAR")
}

func TestMinioUnit_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Create Docker network
	network, err := testcontainers.GenericNetwork(ctx, testcontainers.GenericNetworkRequest{
		NetworkRequest: testcontainers.NetworkRequest{
			Name: "test-network",
		},
	})
	require.NoError(t, err)
	defer network.Remove(ctx)

	// Create Minio unit
	cfg := map[string]any{
		"name":       "test-minio",
		"access_key": "testuser",
		"secret_key": "testpass123",
		"buckets":    []interface{}{"test-bucket", "another-bucket"},
	}

	unit, err := New(cfg)
	require.NoError(t, err)

	minioUnit := unit.(*MinioUnit)

	// Start the unit
	dockerNetwork, ok := network.(*testcontainers.DockerNetwork)
	require.True(t, ok, "Expected DockerNetwork type")

	opts := &e2eframe.UnitStartOptions{
		Network: dockerNetwork,
		Debug:   true,
		Verbose: true,
	}

	err = minioUnit.Start(ctx, opts)
	require.NoError(t, err)
	defer minioUnit.Stop()

	// Wait for ready
	err = minioUnit.WaitForReady(ctx)
	require.NoError(t, err)

	// Test Get methods
	host, err := minioUnit.Get("host")
	require.NoError(t, err)
	assert.NotEmpty(t, host)

	port, err := minioUnit.Get("port")
	require.NoError(t, err)
	assert.NotEmpty(t, port)

	endpoint, err := minioUnit.Get("endpoint")
	require.NoError(t, err)
	assert.NotEmpty(t, endpoint)

	accessKey, err := minioUnit.Get("access_key")
	require.NoError(t, err)
	assert.Equal(t, "testuser", accessKey)

	secretKey, err := minioUnit.Get("secret_key")
	require.NoError(t, err)
	assert.Equal(t, "testpass123", secretKey)

	// Test invalid Get key
	_, err = minioUnit.Get("invalid_key")
	assert.Error(t, err)

	// Test endpoints
	externalEndpoint := minioUnit.ExternalEndpoint()
	assert.NotEmpty(t, externalEndpoint)

	localEndpoint := minioUnit.LocalEndpoint()
	assert.Equal(t, "test-minio:9000", localEndpoint)

	// Test actual Minio connection
	client, err := minio.New(externalEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4("testuser", "testpass123", ""),
		Secure: false,
	})
	require.NoError(t, err)

	// List buckets to verify connection
	buckets, err := client.ListBuckets(ctx)
	require.NoError(t, err)
	assert.Len(t, buckets, 2)

	bucketNames := make([]string, len(buckets))
	for i, bucket := range buckets {
		bucketNames[i] = bucket.Name
	}
	assert.Contains(t, bucketNames, "test-bucket")
	assert.Contains(t, bucketNames, "another-bucket")

	// Test environment variables
	envs := minioUnit.GetEnvRaw(nil)
	assert.Contains(t, envs, "MINIO_ROOT_USER")
	assert.Contains(t, envs, "MINIO_ROOT_PASSWORD")
	assert.Contains(t, envs, "MINIO_ENDPOINT")
	assert.Equal(t, "testuser", envs["MINIO_ROOT_USER"])
	assert.Equal(t, "testpass123", envs["MINIO_ROOT_PASSWORD"])

	// Test SetEnvs
	newEnvs := map[string]string{
		"NEW_VAR": "new_value",
	}
	minioUnit.SetEnvs(newEnvs)
	updatedEnvs := minioUnit.GetEnvRaw(nil)
	assert.Contains(t, updatedEnvs, "NEW_VAR")
	assert.Equal(t, "new_value", updatedEnvs["NEW_VAR"])
}

func TestMinioUnit_RegistrationAndUnmarshalling(t *testing.T) {
	// Test that the unit is properly registered
	assert.True(t, e2eframe.KindExists("minio"))

	yamlContent := `
name: test-minio
image: minio/minio:latest
access_key: testuser
secret_key: testpass123
buckets:
  - bucket1
  - bucket2
`

	var node yaml.Node
	err := yaml.Unmarshal([]byte(yamlContent), &node)
	require.NoError(t, err)

	// Get the document node (first child of root)
	require.Len(t, node.Content, 1)
	// Test unmarshalling through the framework
	unit, err := e2eframe.UnmarshallUnit("minio", node.Content[0])
	require.NoError(t, err)
	assert.NotNil(t, unit)

	minioUnit, ok := unit.(*MinioUnit)
	require.True(t, ok)
	assert.Equal(t, "test-minio", minioUnit.Name())
}

func TestMinioUnit_ErrorCases(t *testing.T) {
	minioUnit := &MinioUnit{}

	// Test Get with unstarted container
	_, err := minioUnit.Get("host")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "container not started")

	// Test WaitForReady with unstarted container
	err = minioUnit.WaitForReady(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "container not started")

	// Test Stop with nil container
	err = minioUnit.Stop()
	assert.NoError(t, err) // Should not error on nil container

	// Test ExternalEndpoint with nil container
	endpoint := minioUnit.ExternalEndpoint()
	assert.Empty(t, endpoint)
}

func TestMinioUnit_ConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name:    "valid minimal config",
			yaml:    `name: test-minio`,
			wantErr: false,
		},
		{
			name:    "invalid yaml structure",
			yaml:    `- invalid: structure`,
			wantErr: true,
		},
		{
			name:    "missing name field",
			yaml:    `image: minio/minio:latest`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var node yaml.Node
			err := yaml.Unmarshal([]byte(tt.yaml), &node)
			require.NoError(t, err)

			// Get the document node (first child of root)
			require.Len(t, node.Content, 1)
			_, err = UnmarshalUnit(node.Content[0])
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
