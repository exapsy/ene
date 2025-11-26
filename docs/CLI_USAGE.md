# ENE CLI Usage Guide

ENE is an end-to-end (e2e) testing framework that uses Docker containers to spin up test environments and validate your applications through HTTP, database, and mocked service interactions.

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [CLI Commands](#cli-commands)
  - [Default Command (Run Tests)](#default-command-run-tests)
  - [scaffold-test](#scaffold-test)
  - [dry-run](#dry-run)
  - [list-suites](#list-suites)
  - [cleanup](#cleanup)
  - [version](#version)
- [Command Options](#command-options)
- [Test Suite Configuration](#test-suite-configuration)
- [Examples](#examples)
- [Advanced Usage](#advanced-usage)

---

## Installation

### Prerequisites

- **Go 1.18+** - Required to build the CLI
- **Docker** - Must be running for test execution
- **mongosh** (optional) - Required only if using MongoDB migrations with JS files

### Building from Source

```bash
# Clone the repository
git clone <repository-url>
cd ene

# Build the CLI
go build -o ene main.go

# Optional: Move to PATH
sudo mv ene /usr/local/bin/
```

---

## Quick Start

### 1. Create a Test Suite

```bash
# Scaffold a new test suite with HTTP mock
ene scaffold-test my-first-test --tmpl=httpmock

# Scaffold with multiple templates
ene scaffold-test my-api-test --tmpl=mongo,http,httpmock
```

This creates a new directory under `./tests/my-first-test/` with a `suite.yml` file.

### 2. Validate Configuration

```bash
# Validate all test suites
ene dry-run --verbose

# Validate a specific test file
ene dry-run tests/my-first-test/suite.yml --verbose
```

### 3. Run Tests

```bash
# Run all tests
ene

# Run specific test suite
ene --suite=my-first-test

# Run with verbose output
ene --suite=my-first-test --verbose

# Run tests in parallel
ene --parallel

# Generate HTML and JSON reports
ene --html=report.html --json=report.json
```

---

## CLI Commands

### Default Command (Run Tests)

When called with no subcommand, `ene` runs all e2e tests.

```bash
ene [flags]
```

### `list-suites`

List all available test suites in the `tests/` directory.

```bash
ene list-suites
```

**Output:**
```
Available test suites:
  user-api-tests
  payment-service-tests
  integration-tests
```

---

### `cleanup`

Clean up orphaned Docker resources (containers, networks) created by ENE tests.

```bash
ene cleanup [resource-type] [flags]
```

**Resource Types:**
- `all` - Clean all resource types (default)
- `networks` - Clean only Docker networks
- `containers` - Clean only Docker containers

**Flags:**
- `--dry-run` - Show what would be removed without actually removing
- `--force` - Skip confirmation prompt
- `--all` - Include all resources, not just orphaned ones
- `--older-than=<duration>` - Only clean resources older than specified duration (e.g., 1h, 30m, 24h)
- `--verbose` - Show detailed information about discovered resources

**Examples:**

```bash
# Interactive cleanup (shows what will be removed and asks for confirmation)
ene cleanup

# Preview what would be removed (safe, no changes made)
ene cleanup --dry-run --verbose

# Force cleanup without confirmation
ene cleanup --force

# Clean only networks
ene cleanup networks --force

# Clean only containers
ene cleanup containers --force

# Clean resources older than 1 hour
ene cleanup --older-than=1h --force

# Clean resources older than 24 hours
ene cleanup --older-than=24h --force

# Include all resources (not just orphaned)
ene cleanup --all --force

# Verbose output for debugging
ene cleanup --verbose --dry-run
```

**Use Cases:**

**CI/CD Integration:**
```yaml
# GitLab CI
after_script:
  - ene cleanup --older-than=30m --force

# GitHub Actions
- name: Cleanup Docker Resources
  if: always()
  run: ene cleanup --older-than=30m --force --verbose
```

**Periodic Cleanup (Cron):**
```bash
# Add to crontab for nightly cleanup
0 2 * * * /usr/local/bin/ene cleanup --older-than=24h --force >> /var/log/ene-cleanup.log 2>&1
```

**Manual Troubleshooting:**
```bash
# Check what's orphaned
ene cleanup --dry-run --verbose

# Clean specific type
ene cleanup containers --force

# Clean everything
ene cleanup --force
```

**How It Works:**

The cleanup command:
1. Discovers orphaned Docker resources created by ENE (identified by `testcontainers-` prefix)
2. Filters by age if `--older-than` is specified
3. Removes containers first, then networks (proper ordering prevents "active endpoints" errors)
4. Reports what was cleaned and any errors encountered

**Troubleshooting:**

If you see "network has active endpoints" errors:
```bash
# Clean containers first, then networks
ene cleanup containers --force
ene cleanup networks --force
```

For more details, see:
- [Cleanup Architecture Guide](../e2eframe/CLEANUP_ARCHITECTURE.md)
- [Migration Guide](../e2eframe/MIGRATION_GUIDE.md)

---

### `version`

Display the ENE version information.

```bash
ene version
```

---

### `scaffold-test`

Create a new test suite scaffold with predefined templates.

```bash
ene scaffold-test <name> [flags]
```

**Arguments:**
- `<name>` - Name of the test suite (required)

**Flags:**
- `--tmpl=<templates>` - Comma-separated list of templates to include
  - Available templates: `mongo`, `http`, `httpmock`
  - Default: `mongo,httpmock`

**Examples:**
```bash
# Create test with default templates
ene scaffold-test user-service-test

# Create test with only HTTP mock
ene scaffold-test api-test --tmpl=httpmock

# Create test with MongoDB and HTTP service
ene scaffold-test integration-test --tmpl=mongo,http
```

### `dry-run`

Validate test configuration without running containers. Useful for CI/CD validation and syntax checking.

```bash
ene dry-run [test-file] [flags]
```

**Arguments:**
- `[test-file]` - Optional path to specific test file to validate

**Flags:**
- `--verbose` / `-v` - Enable detailed validation output
- `--debug` - Enable debug mode with extra information
- `--base-dir=<path>` - Base directory for tests (default: current directory)

**Examples:**
```bash
# Validate all test suites
ene dry-run --verbose

# Validate specific test file
ene dry-run tests/my-test/suite.yml

# Validate with debug information
ene dry-run --debug --verbose
```

### `list-suites`

List all available test suites in the tests directory.

```bash
ene list-suites [flags]
```

**Flags:**
- `--base-dir=<path>` - Base directory for tests (default: current directory)

**Example:**
```bash
ene list-suites
```

**Output:**
```
Available test suites:
  api-tests
  integration-tests
  mock-tests
```

### `version`

Display version information.

```bash
ene version
```

**Output:**
```
ene version dev
commit: unknown
built: unknown
```

### `completion`

Generate shell completion scripts for bash, zsh, fish, or PowerShell.

```bash
ene completion <shell>
```

**Examples:**
```bash
# Bash completion
ene completion bash > /etc/bash_completion.d/ene

# Zsh completion
ene completion zsh > "${fpath[1]}/_ene"

# Fish completion
ene completion fish > ~/.config/fish/completions/ene.fish

# PowerShell completion
ene completion powershell > ene.ps1
```

---

## Command Options

### Global Flags (for test execution)

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--verbose` / `-v` | bool | false | Enable detailed logs including container output and HTTP request/response details |
| `--pretty` | bool | true | Pretty print output with colors and formatting |
| `--debug` | bool | false | Enable debug mode with extra diagnostic information |
| `--parallel` | bool | false | Run test suites in parallel (faster but more resource-intensive) |
| `--suite=<names>` | string | "" | Run specific test suites (comma-separated), supports partial matching |
| `--html=<path>` | string | "" | Generate HTML report at specified path |
| `--json=<path>` | string | "" | Generate JSON report at specified path |
| `--base-dir=<path>` | string | "" | Base directory for tests (default: current directory) |
| `--cleanup-cache` | bool | false | Cleanup old cached Docker images to prevent bloat |
| `--help` / `-h` | bool | false | Show help information |
| `--version` | bool | false | Show version information |

### Verbose Mode for HTTP Tests

When `--verbose` is enabled, HTTP tests will display detailed request and response information:

**Request Details:**
- HTTP Method (GET, POST, etc.)
- Full URL including query parameters
- All request headers
- Request body payload

**Response Details:**
- HTTP status code
- All response headers
- Response body content

**Example Output:**
```
=== HTTP Request ===
POST http://localhost:8080/api/users?include=profile
Headers:
  Content-Type: application/json
  Authorization: Bearer test-token
Body:
{"name":"John Doe","email":"john@example.com"}
====================

=== HTTP Response ===
Status: 201 Created
Headers:
  Content-Type: application/json
  X-Request-Id: abc123
Body:
{"id":1,"name":"John Doe","email":"john@example.com"}
=====================
```

**Failure Messages:**

When an HTTP test fails, the error message automatically includes full request and response details, regardless of whether verbose mode is enabled. This ensures you have all the information needed to debug the failure:

```
expected status code 200, got 404

=== Request Details ===
GET http://localhost:8080/api/users/999?include=profile
Headers:
  Authorization: Bearer test-token
  Accept: application/json
========================

=== Response Details ===
Status: 404 Not Found
Headers:
  Content-Type: application/json
Body:
{"error":"User not found"}
=========================
```

This is particularly useful for:
- Debugging failing tests
- Understanding API behavior
- Verifying request/response formats
- Troubleshooting authentication issues
- Checking query parameter handling

### Suite Filtering

The `--suite` flag supports multiple patterns:

```bash
# Exact match
ene --suite=api-tests

# Multiple suites
ene --suite=api-tests,mock-tests

# Partial match (prefix)
ene --suite=api-

# Partial match (suffix)
ene --suite=-tests

# Multiple patterns with partial matching
ene --suite=TestService_,_Function
```

---

## Test Suite Configuration

### Directory Structure

```
.
├── tests/
│   ├── my-test-suite/
│   │   ├── suite.yml          # Test configuration (required)
│   │   ├── db.js              # MongoDB migrations (optional)
│   │   ├── Dockerfile         # Custom Dockerfile (optional)
│   │   └── .env               # Environment variables (optional)
│   └── another-test/
│       └── suite.yml
└── ene                         # CLI binary
```

### Suite Configuration File (`suite.yml`)

The `suite.yml` file defines the test suite configuration.

```yaml
kind: e2e_test:v1
name: my-test-suite

# Fixtures: Reusable data values (both array and map formats work)
fixtures:
  - content_type_inline: application/json; charset=utf-8
  - api_key: test-key-12345

# Units: Services/containers to spin up
units:
  # HTTP Mock Server
  - name: mock-api
    kind: httpmock
    app_port: 8080
    routes:
      - path: /healthcheck
        method: GET
        response:
          status: 200
          body:
            status: ok
          headers:
            Content-Type: "{{ content_type_inline }}"

  # MongoDB Database
  - name: mongodb
    kind: mongo
    image: mongo:6.0
    app_port: 27017
    migrations: db.js
    startup_timeout: 30s

  # HTTP Service (your application)
  - name: app
    kind: http
    dockerfile: Dockerfile
    app_port: 8080
    healthcheck: /health
    startup_timeout: 4m
    env_file: .env
    env:
      - DB_DSN={{ mongodb.dsn }}
      - API_KEY={{ api_key }}

# Target: Which unit to send test requests to
target: app

# Tests: Test cases to execute
tests:
  - name: health check
    kind: http
    request:
      path: /health
      method: GET
      timeout: 5s
    expect:
      status_code: 200
      body_asserts:
        status: "ok"
      header_asserts:
        Content-Type:
          present: true
```

### Unit Types

#### 1. HTTP Mock (`httpmock`)

Mocks HTTP endpoints without running actual services.

```yaml
- name: mock-service
  kind: httpmock
  app_port: 8080
  routes:
    - path: /api/users
      method: GET
      response:
        status: 200
        body:
          users: []
        headers:
          Content-Type: application/json
```

#### 2. HTTP Service (`http`)

Runs your actual application in a container.

```yaml
- name: my-app
  kind: http
  dockerfile: Dockerfile     # Build from Dockerfile
  # OR
  image: myapp:latest        # Use existing image
  app_port: 8080
  healthcheck: /health       # Healthcheck endpoint
  startup_timeout: 2m
  env_file: .env
  env:
    - DATABASE_URL={{ mongodb.dsn }}
  cmd:                       # Optional: Override container command
    - ./server
    - --port=8080
```

#### 3. MongoDB (`mongo`)

Runs MongoDB with optional migrations.

```yaml
- name: database
  kind: mongo
  image: mongo:6.0
  app_port: 27017
  database: testdb
  user: testuser
  password: testpass
  migrations: db.js          # JavaScript migration file
  startup_timeout: 30s
```

#### 4. MinIO (`minio`)

Runs MinIO object storage for S3-compatible testing.

```yaml
- name: storage
  kind: minio
  image: minio/minio:latest
  access_key: testuser
  secret_key: testpass123
  app_port: 9000
  console_port: 9001
  startup_timeout: 30s
  buckets:
    - uploads
    - processed
```

#### 5. PostgreSQL (`postgres`)

Runs PostgreSQL database with migrations.

```yaml
- name: postgres
  kind: postgres
  image: postgres:14
  app_port: 5432
  database: testdb
  user: testuser
  password: testpass
  migrations: migrations/    # SQL migration files
  startup_timeout: 30s
```

### Test Types

#### HTTP Test

```yaml
- name: create user
  kind: http
  request:
    method: POST
    path: /api/users
    timeout: 5s
    headers:
      Content-Type: application/json
      Authorization: Bearer {{ api_token }}
    body:
      name: John Doe
      email: john@example.com
    query_params:
      include: profile
  expect:
    status_code: 201
    body_asserts:
      id:
        present: true
      name: John Doe
      email:
        matches: "^[a-z0-9._%+-]+@[a-z0-9.-]+\\.[a-z]{2,}$"
    header_asserts:
      Location:
        present: true
        contains: /api/users/
```

#### PostgreSQL Test

```yaml
- name: verify user count
  kind: postgres
  query: "SELECT COUNT(*) as count FROM users WHERE status = 'active'"
  expect:
    row_count: 1
    column_values:
      count: 5

- name: check user data
  kind: postgres
  query: "SELECT id, name, email FROM users WHERE id = 1"
  expect:
    rows:
      - id: 1
        name: "Alice"
        email: "alice@example.com"

- name: no orphaned orders
  kind: postgres
  query: "SELECT * FROM orders WHERE user_id NOT IN (SELECT id FROM users)"
  expect:
    no_rows: true
```

See [POSTGRES_TESTS.md](./POSTGRES_TESTS.md) for more examples.

#### MongoDB Test

```yaml
- name: check active users
  kind: mongo
  collection: users
  filter:
    status: active
    age:
      $gte: 18
  expect:
    document_count: 5

- name: aggregate by role
  kind: mongo
  collection: users
  pipeline:
    - $group:
        _id: "$role"
        count: { $sum: 1 }
    - $sort:
        count: -1
  expect:
    min_document_count: 2

- name: verify user details
  kind: mongo
  collection: users
  filter:
    _id: 1
  expect:
    documents:
      - _id: 1
        name: "Alice"
        email: "alice@example.com"
```

See [MONGO_QUICK_REFERENCE.md](./MONGO_QUICK_REFERENCE.md) for more examples.

#### MinIO Test

```yaml
- name: verify file uploaded
  kind: minio
  verify_state:
    files_exist:
      - uploads/user123/profile.jpg
      - uploads/user123/resume.pdf
    bucket_counts:
      uploads: 2
      processed: 0
    required:
      buckets:
        uploads:
          - path: user123/profile.jpg
            min_size: 10B
            max_size: 10MB
            content_type: image/jpeg
            max_age: 5m
```

### Assertions

#### Body Assertions

```yaml
body_asserts:
  # String shorthand for equals
  data.user.name: "John Doe"
  
  # Explicit assertions with object
  data.user.email:
    contains: "John"            # Substring match
    not_contains: "Admin"       # Substring not present
    matches: "^John.*"          # Regex match
    not_matches: "^Admin.*"     # Regex not match
    present: true               # Field exists
    type: string                # Type check (string, int, float, bool, array, object)
  
  # Comparison operators with symbols
  data.user.age:
    ">": 10                     # greater_than (numeric comparison)
    "<": 100                    # less_than (numeric comparison)
  
  # Length checking
  data.user.tags:
    length: 5                   # Array/string length (or 'size')
```

#### Header Assertions

```yaml
header_asserts:
  # String shorthand for equals
  Content-Type: application/json
  
  # Explicit assertions with object
  X-Custom-Header:
    contains: json
    not_contains: xml
    matches: "^application/.*"
    not_matches: "^text/.*"
    present: true
```

### Fixtures and Interpolation

Fixtures allow you to define reusable values that can be interpolated in your test configuration.

```yaml
fixtures:
  - api_base: http://api.example.com
  - auth_token: Bearer abc123
  - test_data: { file: ./testdata/user.json }  # Load from file

# Use fixtures with {{ fixture_name }}
tests:
  - name: test with fixtures
    kind: http
    request:
      path: "{{ api_base }}/users"
      headers:
        Authorization: "{{ auth_token }}"
      body: "{{ test_data }}"
```

**Service Variable Interpolation:**

Access service properties using `{{ service_name.property }}`:

```yaml
env:
  - DATABASE_URL={{ mongodb.dsn }}
  - MONGO_HOST={{ mongodb.host }}
  - MONGO_PORT={{ mongodb.port }}
  - REDIS_URL={{ redis.endpoint }}
  - MINIO_ENDPOINT={{ storage.local_endpoint }}
```

---

## Examples

### Example 1: Simple HTTP Mock Test

```bash
# Create test
ene scaffold-test simple-mock --tmpl=httpmock

# Edit tests/simple-mock/suite.yml
cat > tests/simple-mock/suite.yml <<EOF
kind: e2e_test:v1
name: simple-mock

units:
  - name: api
    kind: httpmock
    app_port: 8080
    routes:
      - path: /ping
        method: GET
        response:
          status: 200
          body:
            message: pong

target: api

tests:
  - name: ping test
    kind: http
    request:
      path: /ping
      method: GET
    expect:
      status_code: 200
      body_asserts:
        message: pong
EOF

# Run test
ene --suite=simple-mock --verbose
```

### Example 2: MongoDB Integration Test

```bash
# Create test with MongoDB
ene scaffold-test db-test --tmpl=mongo,http

# Create migration file
cat > tests/db-test/db.js <<EOF
db.users.insertMany([
  { name: "Alice", email: "alice@example.com" },
  { name: "Bob", email: "bob@example.com" }
]);
EOF

# Run test
ene --suite=db-test --verbose
```

### Example 3: Full Application Test

```bash
# Run all tests in parallel with reports
ene --parallel --html=report.html --json=report.json --verbose

# Run specific suites
ene --suite=api-tests,integration-tests --verbose

# Run with debug information
ene --debug --verbose --suite=failing-test
```

### Example 4: CI/CD Integration

```bash
# Validate configuration in CI
ene dry-run --verbose
if [ $? -ne 0 ]; then
  echo "Test configuration is invalid"
  exit 1
fi

# Run tests with reports
ene --parallel --json=test-results.json --html=test-report.html

# Check exit code
if [ $? -ne 0 ]; then
  echo "Tests failed"
  exit 1
fi
```

---

## Advanced Usage

### Running Tests in CI/CD

**GitHub Actions Example:**

```yaml
name: E2E Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      
      - name: Build ENE
        run: go build -o ene main.go
      
      - name: Validate Configuration
        run: ./ene dry-run --verbose
      
      - name: Run Tests
        run: ./ene --parallel --json=results.json --html=report.html
      
      - name: Upload Test Results
        if: always()
        uses: actions/upload-artifact@v3
        with:
          name: test-reports
          path: |
            results.json
            report.html
```

### Performance Tips

1. **Use Parallel Execution**: For faster test runs with independent test suites
   ```bash
   ene --parallel
   ```

2. **Cache Cleanup**: Prevent Docker image bloat
   ```bash
   ene --cleanup-cache
   ```

3. **Targeted Testing**: Run only specific suites during development
   ```bash
   ene --suite=unit-tests
   ```

4. **Optimize Timeouts**: Adjust startup timeouts based on your services
   ```yaml
   startup_timeout: 30s  # Reduce for faster services
   ```

### Debugging Failed Tests

1. **Enable Verbose Output**:
   ```bash
   ene --verbose --suite=failing-test
   ```

2. **Enable Debug Mode**:
   ```bash
   ene --debug --verbose --suite=failing-test
   ```

3. **Run Without Parallel**:
   ```bash
   ene --suite=failing-test  # Easier to read logs
   ```

4. **Check Docker Containers**:
   ```bash
   docker ps -a  # View running/stopped containers
   docker logs <container-id>  # View container logs
   ```

### Shell Completion

Enable shell completion for better UX:

```bash
# Bash
source <(ene completion bash)

# Zsh
source <(ene completion zsh)

# Fish
ene completion fish | source
```

This enables auto-completion for:
- Commands
- Flags
- Test suite names (for `--suite` flag)

---

## Test Output

### Console Output

```
▶ TEST RUN STARTED
══════════════════════════════════════════════════════════════════════════

[1/3] api-tests
  ⋯  Setting up network...
  ✓  health check 15ms
  ✓  create user 42ms
  ✓  get user 28ms
  (Setup: 1.2s | Tests: 85ms | Overhead: 0ms)

[2/3] integration-tests
  ⋯  Setting up network...
  ✓  database integration 156ms
  (Setup: 2.5s | Tests: 156ms | Overhead: 0ms)

════════════════════════════════════════════════════════════════════════════
SUMMARY                                                               4.2s
  Setup: 3.7s  |  Tests: 241ms  |  Overhead: 260ms

  0 failed  |  4 passed  |  1 skipped
```

### HTML Report

When using `--html=report.html`, generates a comprehensive HTML report with:
- Test summary and statistics
- Individual test results with timing
- Failed test details with error messages
- Pass/fail charts and visualizations

### JSON Report

When using `--json=report.json`, generates machine-readable JSON:

```json
{
  "metadata": {
    "startTime": "2024-01-15T10:30:00Z",
    "endTime": "2024-01-15T10:30:05Z",
    "durationMs": 5000,
    "totalTests": 10,
    "totalPassed": 9,
    "totalFailed": 1,
    "totalSkipped": 0
  },
  "suites": [
    {
      "name": "api-tests",
      "tests": [...]
    }
  ]
}
```

---

## Troubleshooting

### Common Issues

**1. Docker Connection Error**
```
Error: Cannot connect to Docker daemon
```
**Solution**: Ensure Docker is running:
```bash
docker ps
```

**2. Port Already in Use**
```
Error: Port 8080 is already in use
```
**Solution**: Change the port in your `suite.yml` or stop the conflicting service

**3. Container Startup Timeout**
```
Error: Container failed to start within timeout
```
**Solution**: Increase `startup_timeout` in unit configuration:
```yaml
startup_timeout: 2m
```

**4. Test File Not Found**
```
Error: failed to open test suite file
```
**Solution**: Ensure `suite.yml` exists in the test directory:
```bash
ls tests/my-test/suite.yml
```

**5. Invalid Configuration**
```
Configuration validation failed
```
**Solution**: Run dry-run to see specific errors:
```bash
ene dry-run tests/my-test/suite.yml --verbose
```

---

## Environment Variables

ENE respects the following environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `DOCKER_API_VERSION` | Docker API version | 1.45 |
| `DOCKER_HOST` | Docker daemon host | unix:///var/run/docker.sock |

---

## Contributing

To contribute to ENE or report issues, please visit the project repository.

---

## License

[Insert License Information]

---

## Support

For issues, questions, or feature requests, please:
1. Check this documentation
2. Run tests with `--debug --verbose` for detailed output
3. Open an issue on the project repository