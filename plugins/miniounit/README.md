# Minio Unit Plugin

The Minio unit plugin provides support for running Minio object storage containers in the ene E2E testing framework. This plugin allows you to easily set up and test applications that depend on S3-compatible object storage.

## Features

- **Easy Setup**: Start Minio containers with minimal configuration
- **Automatic Bucket Creation**: Pre-create buckets during startup
- **Custom Commands**: Support for custom Minio server commands via `cmd`
- **Environment Variables**: Full support for environment variable configuration
- **Network Integration**: Seamless integration with ene's Docker networking
- **Variable Access**: Access Minio connection details from other units and tests

## Configuration

### Basic Configuration

```yaml
kind: e2e_test:v1
name: minio-test
units:
  - kind: minio
    name: minio-server
    app_port: 9000
    console_port: 9001

target: minio-server
tests:
  - name: "Test Minio health"
    kind: http
    request:
      method: GET
      path: /minio/health/live
    expect:
      status_code: 200
```

### Full Configuration

```yaml
units:
  - kind: minio
    name: minio-server
    image: minio/minio:latest           # Docker image (default: minio/minio:latest)
    access_key: myaccesskey             # Access key (default: minioadmin)
    secret_key: mysecretkey             # Secret key (default: minioadmin)
    app_port: 9000                      # Minio API port (default: 9000)
    console_port: 9001                  # Minio console port (default: 9001)
    startup_timeout: 30s                # Container startup timeout
    buckets:                            # Buckets to create automatically
      - uploads
      - documents
      - images
    cmd:                                # Custom command (optional)
      - server
      - /data
      - --console-address
      - ":9001"
    env:                                # Environment variables
      - MINIO_BROWSER=on
      - MINIO_DOMAIN=localhost
    env_file: .env                      # Load environment from file
```

## Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `name` | string | **required** | Unit name for referencing |
| `image` | string | `minio/minio:latest` | Docker image to use |
| `access_key` | string | `minioadmin` | Minio access key |
| `secret_key` | string | `minioadmin` | Minio secret key |
| `app_port` | int | `9000` | Minio API port |
| `console_port` | int | `9001` | Minio console port |
| `startup_timeout` | duration | `10s` | Container startup timeout |
| `buckets` | []string | `[]` | Buckets to create on startup |
| `cmd` | []string | `["server", "/data", "--console-address", ":9001"]` | Custom command |
| `env` | []string | `[]` | Environment variables |
| `env_file` | string | `""` | Environment file path |

## Variable Access

The Minio plugin exposes several variables that can be accessed from other units and tests:

| Variable | Description | Example |
|----------|-------------|---------|
| `host` | Container hostname | `{{ minio-server.host }}` |
| `port` | External API port | `{{ minio-server.port }}` |
| `endpoint` | External endpoint | `{{ minio-server.endpoint }}` |
| `local_endpoint` | Internal endpoint | `{{ minio-server.local_endpoint }}` |
| `access_key` | Access key | `{{ minio-server.access_key }}` |
| `secret_key` | Secret key | `{{ minio-server.secret_key }}` |
| `console_port` | External console port | `{{ minio-server.console_port }}` |
| `console_endpoint` | Console endpoint | `{{ minio-server.console_endpoint }}` |

## Examples

### Basic File Upload Test

```yaml
kind: e2e_test:v1
name: file-upload-test
units:
  - kind: minio
    name: storage
    buckets:
      - uploads

  - kind: http
    name: app
    image: node:18-alpine
    app_port: 3000
    env:
      - MINIO_ENDPOINT={{ storage.local_endpoint }}
      - MINIO_ACCESS_KEY={{ storage.access_key }}
      - MINIO_SECRET_KEY={{ storage.secret_key }}
    cmd:
      - /bin/sh
      - -c
      - |
        npm init -y
        npm install express minio
        cat > server.js << 'EOF'
        const express = require('express');
        const Minio = require('minio');
        const app = express();
        
        const minioClient = new Minio.Client({
          endPoint: process.env.MINIO_ENDPOINT.split(':')[0],
          port: parseInt(process.env.MINIO_ENDPOINT.split(':')[1]),
          useSSL: false,
          accessKey: process.env.MINIO_ACCESS_KEY,
          secretKey: process.env.MINIO_SECRET_KEY
        });
        
        app.post('/upload/:filename', (req, res) => {
          minioClient.putObject('uploads', req.params.filename, 'test content')
            .then(() => res.json({success: true}))
            .catch(err => res.status(500).json({error: err.message}));
        });
        
        app.listen(3000, () => console.log('Server ready'));
        EOF
        node server.js

target: app
tests:
  - name: "Upload file"
    kind: http
    request:
      method: POST
      path: /upload/test.txt
    expect:
      status_code: 200
```

### Custom Minio Configuration

```yaml
units:
  - kind: minio
    name: minio-custom
    access_key: admin
    secret_key: password123
    buckets:
      - data
      - backups
    cmd:
      - server
      - /data
      - --console-address
      - ":9001"
      - --address
      - ":9000"
    env:
      - MINIO_BROWSER=on
      - MINIO_PROMETHEUS_AUTH_TYPE=public
```

### Multi-Bucket Setup

```yaml
units:
  - kind: minio
    name: storage
    buckets:
      - user-uploads
      - system-data
      - temp-files
      - backups
    env:
      - MINIO_BROWSER=on
      - MINIO_DOMAIN=localhost
```

## Environment Variables

The Minio plugin automatically sets the following environment variables:

- `MINIO_ROOT_USER`: Access key for authentication
- `MINIO_ROOT_PASSWORD`: Secret key for authentication
- `MINIO_ENDPOINT`: Internal endpoint for container communication

Additional environment variables can be set via the `env` array or `env_file` option.

## Bucket Management

Buckets specified in the `buckets` array are automatically created during container startup. This ensures your tests can immediately use the storage without additional setup steps.

## Integration with Applications

Use the exposed variables to configure your application containers:

```yaml
units:
  - kind: minio
    name: storage
    
  - kind: http
    name: api
    env:
      - S3_ENDPOINT={{ storage.local_endpoint }}
      - S3_ACCESS_KEY={{ storage.access_key }}
      - S3_SECRET_KEY={{ storage.secret_key }}
      - S3_BUCKET=uploads
```

## Testing Best Practices

1. **Use meaningful bucket names**: Choose bucket names that reflect their purpose
2. **Test both success and error cases**: Verify error handling for missing files, permissions, etc.
3. **Clean up resources**: Rely on ene's automatic container cleanup
4. **Use local endpoints**: For container-to-container communication, use `local_endpoint`
5. **Test file operations**: Upload, download, list, and delete operations

## Troubleshooting

### Container Won't Start

- Check the `startup_timeout` if containers take longer to initialize
- Verify the Docker image is available
- Check for port conflicts

### Connection Issues

- Ensure you're using the correct endpoint (`local_endpoint` for internal, `endpoint` for external)
- Verify access credentials are correctly configured
- Check network connectivity between containers

### Bucket Creation Fails

- Verify bucket names are valid (lowercase, no special characters)
- Check Minio logs for detailed error messages
- Ensure sufficient startup time for bucket creation

## Development

The Minio plugin is located in `ene/plugins/miniounit/` and includes:

- `unit.go`: Main plugin implementation
- `unit_test.go`: Comprehensive test suite
- Integration with the ene framework's unit interface

To contribute or modify the plugin, ensure all tests pass:

```bash
cd ene
go test ./plugins/miniounit/... -v
```
