# ENE Quick Reference

A quick reference cheat sheet for common ENE commands and configurations.

## CLI Commands

```bash
# Run all tests
ene

# Run specific suite
ene --suite=my-test

# Run multiple suites
ene --suite=test1,test2

# Run with verbose output
ene -v

# Run in parallel
ene --parallel

# Generate reports
ene --html=report.html --json=report.json

# Validate configuration
ene dry-run

# Validate specific file
ene dry-run tests/my-test/suite.yml

# List all suites
ene list-suites

# Create new test
ene scaffold-test my-test

# Create with specific templates
ene scaffold-test my-test --tmpl=mongo,http

# Show version
ene version

# Shell completion
ene completion bash > /etc/bash_completion.d/ene

# Cleanup orphaned resources
ene cleanup --dry-run              # Preview
ene cleanup --force                # Clean up
ene cleanup --older-than=1h        # Age filter
ene cleanup networks --force       # Networks only
ene cleanup containers --force     # Containers only
```

## Common Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--verbose` | `-v` | Detailed logs |
| `--pretty` | | Pretty output (default: true) |
| `--debug` | | Debug mode |
| `--parallel` | | Run tests in parallel |
| `--suite` | | Filter test suites |
| `--html` | | HTML report path |
| `--json` | | JSON report path |
| `--base-dir` | | Base directory |
| `--cleanup-cache` | | Cleanup Docker cache |

## Cleanup Commands

```bash
# Interactive cleanup (with confirmation)
ene cleanup

# Preview without removing (safe)
ene cleanup --dry-run --verbose

# Force cleanup without confirmation
ene cleanup --force

# Clean specific resource types
ene cleanup networks --force
ene cleanup containers --force

# Age-based filtering
ene cleanup --older-than=1h --force
ene cleanup --older-than=24h --force

# Include all resources (not just orphaned)
ene cleanup --all --force

# Verbose output
ene cleanup --verbose --dry-run
```

### Cleanup Flags

| Flag | Description |
|------|-------------|
| `--dry-run` | Show what would be removed without removing |
| `--force` | Skip confirmation prompt |
| `--all` | Include all resources, not just orphaned |
| `--older-than` | Only clean resources older than duration (e.g., 1h, 30m, 24h) |
| `--verbose` | Show detailed information |

### CI/CD Integration

```bash
# GitLab CI / GitHub Actions
after_script:
  - ene cleanup --older-than=30m --force

# Cron job (nightly cleanup)
0 2 * * * /usr/local/bin/ene cleanup --older-than=24h --force
```

## Basic Configuration

```yaml
kind: e2e_test:v1
name: my-test

fixtures:
  - name: api_key
    value: test-123

units:
  - name: app
    kind: http
    app_port: 8080
    dockerfile: Dockerfile

target: app

tests:
  - name: test
    kind: http
    request:
      path: /health
      method: GET
    expect:
      status_code: 200
```

## Unit Types

### HTTP Mock
```yaml
- name: mock
  kind: httpmock
  app_port: 8080
  routes:
    - path: /api/users
      method: GET
      response:
        status: 200
        body: { users: [] }
```

### HTTP Service
```yaml
- name: app
  kind: http
  dockerfile: Dockerfile
  # OR
  image: myapp:latest
  app_port: 8080
  healthcheck: /health
  startup_timeout: 2m
  env:
    - KEY=value
```

### MongoDB
```yaml
- name: mongodb
  kind: mongo
  image: mongo:6.0
  app_port: 27017
  database: testdb
  user: testuser
  password: testpass
  migrations: db.js
```

### PostgreSQL
```yaml
- name: postgres
  kind: postgres
  image: postgres:14
  app_port: 5432
  database: testdb
  user: testuser
  password: testpass
  migrations: ./migrations
```

### MinIO
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

## HTTP Tests

```yaml
- name: create user
  kind: http
  timeout: 10s
  request:
    method: POST
    path: /api/users
    headers:
      Content-Type: application/json
      Authorization: Bearer {{ token }}
    query_params:
      include: profile
    body:
      name: John Doe
      email: john@example.com
  expect:
    status_code: 201
    body_asserts:
      id:
        present: true
      name: John Doe
    header_asserts:
      Location:
        present: true
```

## Body Assertions

```yaml
body_asserts:
  # Exact match (shorthand)
  status: ok
  
  # Not equal
  error:
    not_equals: failed
  
  # Contains
  message:
    contains: success
  
  # Regex match
  email:
    matches: "^[a-z0-9._%+-]+@[a-z0-9.-]+\\.[a-z]{2,}$"
  
  # Presence
  id:
    present: true
  
  # Type check
  items:
    type: array
  
  # Numeric comparison (symbols)
  age:
    ">": 18
    "<": 100
  
  # Size check
  tags:
    length: 3
  
  # Array containment - check if array contains item matching conditions
  products:
    contains_where:
      name: iPhone
      price:
        ">": 900
  
  # All items must match
  items:
    all_match:
      active: true
  
  # No items should match
  errors:
    none_match:
      critical: true
```

## Header Assertions

```yaml
header_asserts:
  # Exact match (shorthand)
  Content-Type: application/json
  
  # Contains
  Set-Cookie:
    contains: HttpOnly
  
  # Regex
  X-Request-ID:
    matches: "^[0-9a-f-]{36}$"
  
  # Presence
  Authorization:
    present: true
```

## MinIO Tests

```yaml
- name: verify storage
  kind: minio
  verify_state:
    # Simple existence check
    files_exist:
      - uploads/file1.txt
      - uploads/file2.pdf
    
    # Bucket counts
    bucket_counts:
      uploads: 2
      processed: 0
    
    # Required files with specs
    required:
      buckets:
        uploads:
          - path: file1.txt
            min_size: 10B
            max_size: 10MB
            content_type: text/plain
            max_age: 5m
    
    # Forbidden patterns
    forbidden:
      buckets:
        uploads:
          - "*.tmp"
          - "temp_*"
    
    # Constraints
    constraints:
      - bucket: uploads
        file_count: 2
        max_total_size: 100MB
```

## Fixtures

```yaml
# Inline value
fixtures:
  - name: api_key
    value: test-key-123

# From file
fixtures:
  - name: test_data
    file: ./testdata/data.json

# Usage
tests:
  - name: test
    kind: http
    request:
      headers:
        Authorization: "Bearer {{ api_key }}"
      body: "{{ test_data }}"
```

## Service Variables

```yaml
# MongoDB
{{ mongodb.dsn }}
{{ mongodb.host }}
{{ mongodb.port }}
{{ mongodb.database }}

# PostgreSQL
{{ postgres.dsn }}
{{ postgres.host }}
{{ postgres.port }}
{{ postgres.database }}

# MinIO
{{ storage.endpoint }}
{{ storage.local_endpoint }}
{{ storage.access_key }}
{{ storage.secret_key }}

# HTTP Service
{{ app.host }}
{{ app.port }}
{{ app.endpoint }}
```

## Available Types

**HTTP Methods**: GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS

**Assertion Types**: string, number, boolean, array, object, null

**Comparison Operators**: =, !=, >, <, >=, <=

**Size Units**: B, KB, MB, GB

**Duration Units**: s, m, h, d

## JSONPath Examples

```yaml
body_asserts:
  # Root document
  - path: $
    type: object
  
  # Top-level field
  status: ok
  
  # Nested field
  data.user.email:
    present: true
  
  # Array element
  items[0].name: First
  
  # Array length
  - path: items
    size: 5
```

## Common Patterns

### Health Check Test
```yaml
- name: health check
  kind: http
  request:
    path: /health
    method: GET
  expect:
    status_code: 200
```

### CRUD Operations
```yaml
# Create
- name: create
  kind: http
  request:
    method: POST
    path: /api/users
    body: { name: "John" }
  expect:
    status_code: 201

# Read
- name: get
  kind: http
  request:
    method: GET
    path: /api/users/123
  expect:
    status_code: 200

# Update
- name: update
  kind: http
  request:
    method: PUT
    path: /api/users/123
    body: { name: "Jane" }
  expect:
    status_code: 200

# Delete
- name: delete
  kind: http
  request:
    method: DELETE
    path: /api/users/123
  expect:
    status_code: 204
```

### Database Integration
```yaml
units:
  - name: db
    kind: mongo
    image: mongo:6.0
    migrations: db.js
  
  - name: app
    kind: http
    env:
      - DATABASE_URL={{ db.dsn }}
```

### Multiple Services
```yaml
units:
  - name: database
    kind: mongo
  
  - name: storage
    kind: minio
  
  - name: api
    kind: http
    env:
      - DB_URL={{ database.dsn }}
      - S3_ENDPOINT={{ storage.local_endpoint }}
  
  - name: worker
    kind: http
    env:
      - DB_URL={{ database.dsn }}
```

## Error Messages

When assertions fail, ENE provides detailed error messages showing both expected and actual values:

**Header Assertion Errors:**
```
✗ header "Content-Type": expected "application/json" but got "text/plain"
✗ header "Cache-Control" does not contain "no-cache" (got: "public, max-age=3600")
```

**Body Assertion Errors:**
```
✗ expected "John Doe" but got "Jane Smith"
✗ expected value > 18 but got 15
✗ expected array size 5 but got 3
✗ expected type 'string' but got type 'number' at path: user.age (value: "25")
```

These messages help you quickly identify what was expected versus what was actually received.

## Debugging Tips

```bash
# Enable verbose logging
ene --verbose

# Enable debug mode
ene --debug --verbose

# Run specific test
ene --suite=failing-test

# Validate without running
ene dry-run --verbose

# Check Docker containers
docker ps -a
docker logs <container-id>
```

## Exit Codes

- `0` - All tests passed
- `1` - One or more tests failed or error occurred

## Environment Variables

- `DOCKER_API_VERSION` - Docker API version (default: 1.45)
- `DOCKER_HOST` - Docker daemon host

## File Locations

- Test suites: `./tests/<suite-name>/suite.yml`
- Environment files: `./tests/<suite-name>/.env`
- Migration files: `./tests/<suite-name>/db.js`
- Dockerfiles: `./tests/<suite-name>/Dockerfile`

## Common Issues

**Port in use**: Change `app_port` in configuration

**Timeout**: Increase `startup_timeout`

**Docker error**: Ensure Docker is running (`docker ps`)

**Invalid config**: Run `ene dry-run --verbose`

**Missing file**: Check relative paths from suite directory

## Links

- CLI Usage: `CLI_USAGE.md`
- Configuration Reference: `CONFIGURATION_REFERENCE.md`
- Examples: `../examples/`
