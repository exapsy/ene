<img src="./logo.svg" alt="ENE Logo" width="100">

# ENE - End-to-End Testing Framework

[![Go Version](https://img.shields.io/badge/Go-1.25%2B-blue.svg)](https://golang.org)
[![Docker](https://img.shields.io/badge/Docker-Required-blue.svg)](https://docker.com)

ENE is a powerful Docker-based end-to-end testing framework that spins up complete test environments with databases, services, and mocked APIs to validate your applications through comprehensive integration tests.

![Demo](./docs/assets/demo.gif)

## 🌟 Key Features

- **🐳 Docker-Native**: Automatically manages containers for services, databases, and mocks
- **🔧 Multiple Service Types**: HTTP servers, MongoDB, PostgreSQL, MinIO, HTTP mocks
- **📝 Simple YAML Configuration**: Declarative test definitions with fixtures and assertions
- **🔄 Variable Interpolation**: Reuse values across tests with fixtures and service variables
- **📊 Rich Assertions**: JSON path queries, header checks, MinIO state verification
- **🎯 Test Isolation**: Each suite runs in its own Docker network
- **📈 Detailed Reports**: HTML and JSON output formats
- **⚡ Parallel Execution**: Run multiple test suites concurrently
- **🛠️ Easy Scaffolding**: Generate new test suites with templates

## 📋 Prerequisites

- **Docker** (with daemon running)
- **Docker Compose** (for orchestration)
- **Go 1.25+** (to build from source)

### macOS/Colima Users
If using Colima on macOS, set these environment variables:
```bash
export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
export DOCKER_HOST="unix://${HOME}/.colima/docker.sock"
```

## 🚀 Quick Start

### Installation

```bash
# Clone and build
git clone https://github.com/exapsy/ene
cd ene
go build -o ene .

# Optional: Install globally
sudo mv ene /usr/local/bin/

# Verify installation
ene version
```

### Your First Test

```bash
# Create a new test suite
ene scaffold-test my-first-test --tmpl=httpmock

# This creates: ./tests/my-first-test/suite.yml
```

Edit `tests/my-first-test/suite.yml`:

```yaml
kind: e2e_test:v1
name: my-first-test

fixtures:
  - api_key: test-key-123

units:
  - name: api
    kind: httpmock
    app_port: 8080
    routes:
      - path: /health
        method: GET
        response:
          status: 200
          body:
            status: ok

target: api

tests:
  - name: health check
    kind: http
    request:
      path: /health
      method: GET
      headers:
        Authorization: Bearer {{ api_key }}
    expect:
      status_code: 200
      body_asserts:
        status: ok
```

Run your test:

```bash
# Validate configuration
ene dry-run

# Run the test
ene --verbose

# Run with HTML report
ene --html=report.html
```

## 📁 Project Structure

ENE expects a specific directory structure:

```
your-project/
└── tests/                    # Test suites directory (required)
    ├── suite-name-1/
    │   ├── suite.yml        # Test configuration (required)
    │   ├── Dockerfile       # Optional service dockerfile
    │   ├── .env             # Optional environment variables
    │   ├── db.js            # Optional MongoDB migrations
    │   └── migrations/      # Optional PostgreSQL migrations
    └── suite-name-2/
        └── suite.yml
```

### Important: Running ENE

**✅ Run ENE from your project root** (the directory containing `tests/`):
```bash
cd your-project
ene                    # Discovers tests/ automatically
```

**❌ Don't run from inside tests/ directory**:
```bash
cd your-project/tests
ene                    # ERROR: looks for tests/tests/
```

**✅ Use --base-dir if running from elsewhere**:
```bash
cd /anywhere
ene --base-dir=/path/to/your-project
```

## 🎯 Common Commands

```bash
# Run all tests
ene

# Run with verbose output
ene --verbose

# Run specific suite(s)
ene --suite=my-test
ene --suite=test1,test2

# Run tests matching pattern
ene --suite=user_,_api

# Run in parallel
ene --parallel

# Validate without running
ene dry-run
ene dry-run tests/my-test/suite.yml

# Generate reports
ene --html=report.html --json=report.json

# List all test suites
ene list-suites

# Create new test suite
ene scaffold-test my-new-test
ene scaffold-test api-test --tmpl=postgres,http

# Enable debug mode
ene --debug --verbose

# Cleanup old Docker images
ene --cleanup-cache

# Cleanup orphaned Docker resources
ene cleanup --dry-run            # Preview what would be removed
ene cleanup --force              # Remove orphaned resources
ene cleanup --older-than=1h      # Remove resources older than 1 hour
```

## 📝 Configuration Reference

### Suite Configuration (suite.yml)

```yaml
kind: e2e_test:v1              # Required: Version identifier
name: my-test-suite            # Required: Suite name

# Optional: Reusable values
fixtures:
  - user_id: 12345            # Simple value
  - enabled: true             # Boolean
  - api_key: test-key-xyz     # String
  - test_data:                # File-based fixture
      file: ./data/user.json

# Required: Service containers
units:
  - name: postgres
    kind: postgres
    image: postgres:14
    app_port: 5432
    database: testdb
    user: testuser
    password: testpass
    migrations: ./migrations
  
  - name: app
    kind: http
    dockerfile: ./Dockerfile
    app_port: 8080
    healthcheck: /health
    build_timeout: 45s
    startup_timeout: 30s
    env:
      - DATABASE_URL={{ postgres.dsn }}
      - API_KEY={{ api_key }}

# Required: Default target service
target: app

# Required: Test cases
tests:
  - name: create user
    kind: http
    request:
      method: POST
      path: /api/users
      headers:
        Content-Type: application/json
      body:
        name: John Doe
        email: john@example.com
    expect:
      status_code: 201
      body_asserts:
        id:
          present: true
        name: John Doe
```

### Supported Unit Types

#### HTTP Service
```yaml
- name: app
  kind: http
  dockerfile: Dockerfile    # OR image: myapp:latest
  app_port: 8080
  healthcheck: /health      # Optional health endpoint
  build_timeout: 45s        # Time allowed for building Docker image (default: 45s)
  startup_timeout: 30s      # Time allowed for service to become healthy (default: 30s)
  env:
    - KEY=value
  cmd:                      # Optional command override
    - ./app
    - --port=8080
```

**Timeouts Explained:**
- `build_timeout`: Maximum time for Docker image build (downloading dependencies, compilation)
- `startup_timeout`: Maximum time for container startup and health check after build completes

#### HTTP Mock
```yaml
- name: mock-api
  kind: httpmock
  app_port: 8080
  routes:
    - path: /api/users
      method: GET
      response:
        status: 200
        delay: 100ms        # Optional delay
        body:
          users: []
        headers:
          Content-Type: application/json
```

#### PostgreSQL
```yaml
- name: postgres
  kind: postgres
  image: postgres:14
  app_port: 5432
  database: testdb
  user: testuser
  password: testpass
  migrations: ./migrations  # Directory with .sql files
  startup_timeout: 30s
```

#### MongoDB
```yaml
- name: mongodb
  kind: mongo
  image: mongo:6.0
  app_port: 27017
  database: testdb
  user: testuser
  password: testpass
  migrations: db.js         # JavaScript migration file
  startup_timeout: 30s
```

#### MinIO
```yaml
- name: storage
  kind: minio
  image: minio/minio:latest
  access_key: testkey
  secret_key: testsecret
  app_port: 9000
  console_port: 9001
  buckets:
    - uploads
    - processed
```

### Fixtures

**Simple Values (Primitives):**
```yaml
fixtures:
  - api_key: test-key-123
  - user_id: 5432
  - enabled: true
  - description: |
      Multi-line
      text value
```

**File-Based Fixtures:**
```yaml
fixtures:
  - test_payload:
      file: ./testdata/payload.json
  - large_data:
      file: ./testdata/users.json
```

**Usage in Tests:**
```yaml
tests:
  - name: test
    kind: http
    request:
      path: /users/{{ user_id }}
      headers:
        Authorization: Bearer {{ api_key }}
      body: "{{ test_payload }}"
```

### Service Variables

Access service connection details:

```yaml
# PostgreSQL
{{ postgres.dsn }}         # postgresql://user:pass@host:port/db
{{ postgres.host }}
{{ postgres.port }}
{{ postgres.database }}

# MongoDB
{{ mongodb.dsn }}          # mongodb://user:pass@host:port/db
{{ mongodb.host }}
{{ mongodb.port }}
{{ mongodb.database }}

# MinIO
{{ storage.endpoint }}     # External endpoint
{{ storage.local_endpoint }} # Internal Docker endpoint
{{ storage.access_key }}
{{ storage.secret_key }}

# HTTP Service
{{ app.host }}
{{ app.port }}
{{ app.endpoint }}         # http://app:8080
```

### Assertions

**Body Assertions (JSON Path):**
```yaml
body_asserts:
  # Simple equality (shorthand)
  status: ok
  
  # Detailed assertions
  user.id:
    present: true           # Key exists
    type: string           # Type check
  
  user.age:
    ">": 18                # Numeric comparison
    "<": 100
  
  items:
    length: 5              # Array length
    type: array
  
  # Array containment
  products:
    contains_where:
      name: iPhone
      price:
        ">": 900
  
  # All items must match
  users:
    all_match:
      active: true
  
  # No items should match
  errors:
    none_match:
      critical: true
```

**Header Assertions:**
```yaml
header_asserts:
  Content-Type: application/json  # Simple equality
  
  X-Request-ID:
    present: true
    matches: "^[0-9a-f-]{36}$"
  
  Cache-Control:
    contains: no-cache
```

**MinIO State Verification:**
```yaml
- name: verify upload
  kind: minio
  verify_state:
    files_exist:
      - uploads/file1.txt
    
    bucket_counts:
      uploads: 2
    
    required:
      buckets:
        uploads:
          - path: file1.txt
            min_size: 10B
            max_size: 10MB
            content_type: text/plain
            max_age: 5m
    
    forbidden:
      buckets:
        uploads:
          - "*.tmp"
```

## 🧹 Resource Management & Cleanup

ENE automatically manages Docker resources (containers, networks) during test execution. However, if tests are interrupted or fail, resources may be left behind.

### Automatic Cleanup

ENE uses a `CleanupRegistry` that ensures proper cleanup order:
1. **Containers** are removed first
2. **Networks** are removed after containers detach
3. Resources are cleaned even if individual cleanups fail

This prevents common errors like "network has active endpoints."

### Manual Cleanup Command

Use the `ene cleanup` command to remove orphaned resources:

```bash
# Interactive cleanup (shows what will be removed)
ene cleanup

# Preview without removing (dry-run)
ene cleanup --dry-run --verbose

# Force cleanup without confirmation
ene cleanup --force

# Clean specific resource types
ene cleanup networks             # Networks only
ene cleanup containers           # Containers only

# Age-based filtering
ene cleanup --older-than=1h      # Resources older than 1 hour
ene cleanup --older-than=24h     # Resources older than 1 day

# Include all resources (not just orphaned)
ene cleanup --all --force

# Verbose output for debugging
ene cleanup --verbose
```

### Best Practices

**For CI/CD:**
```yaml
# GitLab CI
after_script:
  - ene cleanup --older-than=30m --force
  
# GitHub Actions
- name: Cleanup
  if: always()
  run: ene cleanup --older-than=30m --force
```

**For Local Development:**
```bash
# Check for orphaned resources
ene cleanup --dry-run --verbose

# Clean up after testing
ene cleanup --force
```

**Periodic Cleanup (Cron):**
```bash
# Add to crontab for nightly cleanup
0 2 * * * /usr/local/bin/ene cleanup --older-than=24h --force
```

### Troubleshooting Resource Leaks

If you see orphaned Docker resources:

```bash
# 1. Discover what's orphaned
ene cleanup --dry-run --verbose

# 2. Check Docker resources manually
docker network ls | grep testcontainers
docker ps -a | grep testcontainers

# 3. Clean them up
ene cleanup --force

# 4. For stubborn resources, inspect and remove manually
docker network inspect <network-id>
docker rm -f <container-id>
docker network rm <network-id>
```

For more details, see:
- [Cleanup Architecture Guide](e2eframe/CLEANUP_ARCHITECTURE.md)
- [Migration Guide](e2eframe/MIGRATION_GUIDE.md)

## 🔍 Debugging

### Enable Verbose Logging
```bash
ene --verbose --debug
```

### Check Docker Containers
```bash
docker ps -a
docker logs <container-id>
```

### Validate Configuration
```bash
ene dry-run --verbose
```

### Common Issues

**Port Already in Use:**
- Change `app_port` in your suite.yml
- Check for conflicting services: `lsof -i :8080`

**Startup Timeout:**
- Increase `startup_timeout` for the unit
- Check service logs for errors

**Test Failures:**
- Use `--verbose` to see detailed request/response
- Check assertion paths match your JSON structure
- Verify fixtures are interpolated correctly

## 📚 Documentation

- **[Quick Reference](docs/QUICK_REFERENCE.md)** - Cheat sheet for common patterns
- **[CLI Usage](docs/CLI_USAGE.md)** - Complete command-line reference
- **[Configuration Reference](docs/CONFIGURATION_REFERENCE.md)** - Full YAML schema documentation
- **[Examples](docs/EXAMPLES.md)** - Real-world test examples
- **[Cleanup Architecture](e2eframe/CLEANUP_ARCHITECTURE.md)** - Resource management internals
- **[Migration Guide](e2eframe/MIGRATION_GUIDE.md)** - Upgrade to new cleanup system

## 🤝 Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Write tests for new features
4. Submit a pull request

## 📄 License

[Add your license here]

## 🆘 Support

- **Issues**: [GitHub Issues](https://github.com/exapsy/ene/issues)
- **Documentation**: [docs/](docs/)

## 🎯 Roadmap

- [ ] WebSocket testing support
- [ ] gRPC testing support  
- [ ] Redis unit type
- [ ] Kafka unit type
- [ ] Test retry strategies
- [ ] Performance benchmarking
- [ ] Visual test reports

---

**Made with ❤️ for better integration testing**
