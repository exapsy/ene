# ENE Examples and Recipes

Practical examples and recipes for common testing scenarios with ENE.

## Table of Contents

- [Basic Examples](#basic-examples)
- [API Testing](#api-testing)
- [Database Testing](#database-testing)
- [Object Storage Testing](#object-storage-testing)
- [Multi-Service Testing](#multi-service-testing)
- [Advanced Patterns](#advanced-patterns)
- [Resource Management & Cleanup](#resource-management--cleanup)

---

## Basic Examples

### Hello World: Simple HTTP Mock

The simplest possible test - mock an HTTP endpoint and verify the response.

```yaml
kind: e2e_test:v1
name: hello-world

units:
  - name: api
    kind: httpmock
    app_port: 8080
    routes:
      - path: /hello
        method: GET
        response:
          status: 200
          body:
            message: "Hello, World!"

target: api

tests:
  - name: test hello endpoint
    kind: http
    request:
      path: /hello
      method: GET
    expect:
      status_code: 200
      body_asserts:
        message: "Hello, World!"
```

**Run it:**
```bash
ene scaffold-test hello-world --tmpl=httpmock
# Edit suite.yml with above content
ene --suite=hello-world
```

---

## API Testing

### CRUD Operations

Complete Create, Read, Update, Delete test suite.

```yaml
kind: e2e_test:v1
name: user-crud

fixtures:
  - api_version: v1
  - user_id: "12345"

units:
  - name: api
    kind: httpmock
    app_port: 8080
    routes:
      # Create
      - path: /v1/users
        method: POST
        response:
          status: 201
          body:
            id: "12345"
            name: "John Doe"
            email: "john@example.com"
          headers:
            Location: "/v1/users/12345"
      
      # Read
      - path: /v1/users/12345
        method: GET
        response:
          status: 200
          body:
            id: "12345"
            name: "John Doe"
            email: "john@example.com"
      
      # Update
      - path: /v1/users/12345
        method: PUT
        response:
          status: 200
          body:
            id: "12345"
            name: "Jane Doe"
            email: "jane@example.com"
      
      # Delete
      - path: /v1/users/12345
        method: DELETE
        response:
          status: 204

target: api

tests:
  - name: create user
    kind: http
    request:
      method: POST
      path: /{{ api_version }}/users
      headers:
        Content-Type: application/json
      body:
        name: "John Doe"
        email: "john@example.com"
    expect:
      status_code: 201
      body_asserts:
        id:
          present: true
        name: "John Doe"
      header_asserts:
        Location:
          contains: /users/

  - name: get user
    kind: http
    request:
      method: GET
      path: /v1/users/{{ user_id }}
    expect:
      status_code: 200
      body_asserts:
        id: "{{ user_id }}"
        email:
          matches: "^[a-z0-9._%+-]+@[a-z0-9.-]+\\.[a-z]{2,}$"

  - name: update user
    kind: http
    request:
      method: PUT
      path: /v1/users/{{ user_id }}
      body:
        name: "Jane Doe"
        email: "jane@example.com"
    expect:
      status_code: 200
      body_asserts:
        name: "Jane Doe"

  - name: delete user
    kind: http
    request:
      method: DELETE
      path: /v1/users/{{ user_id }}
    expect:
      status_code: 204
```

### Authentication Flow

Testing authentication and authorization.

```yaml
kind: e2e_test:v1
name: auth-flow

fixtures:
  - username: testuser
  - password: testpass123
  - auth_token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9

units:
  - name: auth-api
    kind: httpmock
    app_port: 8080
    routes:
      - path: /auth/login
        method: POST
        response:
          status: 200
          body:
            token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"
            expires_in: 3600
      
      - path: /auth/me
        method: GET
        response:
          status: 200
          body:
            id: "123"
            username: "testuser"
            role: "admin"
      
      - path: /auth/logout
        method: POST
        response:
          status: 200
          body:
            message: "Logged out successfully"

target: auth-api

tests:
  - name: login successful
    kind: http
    request:
      method: POST
      path: /auth/login
      body:
        username: "{{ username }}"
        password: "{{ password }}"
    expect:
      status_code: 200
      body_asserts:
        token:
          present: true
          type: string
        expires_in:
          ">": 0

  - name: access protected endpoint
    kind: http
    request:
      method: GET
      path: /auth/me
      headers:
        Authorization: "Bearer {{ auth_token }}"
    expect:
      status_code: 200
      body_asserts:
        username: "{{ username }}"

  - name: logout
    kind: http
    request:
      method: POST
      path: /auth/logout
      headers:
        Authorization: "Bearer {{ auth_token }}"
    expect:
      status_code: 200
```

---

## Database Testing

### MongoDB Integration

Testing with MongoDB database and migrations.

```yaml
kind: e2e_test:v1
name: mongo-integration

units:
  - name: mongodb
    kind: mongo
    image: mongo:6.0
    app_port: 27017
    database: testdb
    user: testuser
    password: testpass
    migrations: db.js
    startup_timeout: 30s
  
  - name: api
    kind: http
    image: myapp:latest
    app_port: 8080
    healthcheck: /health
    env:
      - DATABASE_URL={{ mongodb.dsn }}
      - DB_NAME={{ mongodb.database }}

target: api

tests:
  - name: health check includes db
    kind: http
    request:
      path: /health
      method: GET
    expect:
      status_code: 200
      body_asserts:
        database.connected: true
        database.name: testdb

  - name: list users from seed data
    kind: http
    request:
      path: /api/users
      method: GET
    expect:
      status_code: 200
      body_asserts:
        users:
          type: array
          length: 2
```

**Migration file (`db.js`):**
```javascript
// Seed test data
db.users.insertMany([
  {
    name: "Alice",
    email: "alice@example.com",
    created_at: new Date()
  },
  {
    name: "Bob",
    email: "bob@example.com",
    created_at: new Date()
  }
]);

// Create indexes
db.users.createIndex({ email: 1 }, { unique: true });
```

### PostgreSQL with SQL Migrations

```yaml
kind: e2e_test:v1
name: postgres-integration

units:
  - name: postgres
    kind: postgres
    image: postgres:14
    app_port: 5432
    database: testdb
    user: testuser
    password: testpass
    migrations: ./migrations
    startup_timeout: 30s
  
  - name: api
    kind: http
    dockerfile: Dockerfile
    app_port: 8080
    healthcheck: /health
    env:
      - DATABASE_URL={{ postgres.dsn }}

target: api

tests:
  - name: query seeded data
    kind: http
    request:
      path: /api/products
      method: GET
      query_params:
        limit: 10
    expect:
      status_code: 200
      body_asserts:
        products:
          type: array
        total:
          ">": 0
```

**Migration files:**

`migrations/001_create_products.sql`:
```sql
CREATE TABLE products (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    price DECIMAL(10, 2) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

`migrations/002_seed_products.sql`:
```sql
INSERT INTO products (name, price) VALUES
    ('Widget A', 19.99),
    ('Widget B', 29.99),
    ('Widget C', 39.99);
```

---

## Object Storage Testing

### MinIO File Upload and Verification

```yaml
kind: e2e_test:v1
name: storage-test

units:
  - name: storage
    kind: minio
    image: minio/minio:latest
    access_key: testkey
    secret_key: testsecret123
    app_port: 9000
    console_port: 9001
    buckets:
      - uploads
      - processed
    startup_timeout: 30s
  
  - name: file-service
    kind: http
    dockerfile: Dockerfile
    app_port: 8080
    env:
      - S3_ENDPOINT={{ storage.local_endpoint }}
      - S3_ACCESS_KEY={{ storage.access_key }}
      - S3_SECRET_KEY={{ storage.secret_key }}
      - S3_BUCKET=uploads

target: file-service

tests:
  - name: upload file
    kind: http
    request:
      method: POST
      path: /api/upload
      headers:
        Content-Type: multipart/form-data
      body:
        file: test-document.pdf
    expect:
      status_code: 200
      body_asserts:
        uploaded: true
        file_id:
          present: true

  - name: verify file in storage
    kind: minio
    verify_state:
      files_exist:
        - uploads/test-document.pdf
      bucket_counts:
        uploads: 1
        processed: 0
      required:
        buckets:
          uploads:
            - path: test-document.pdf
              min_size: 1B
              max_size: 10MB
              max_age: 1m

  - name: process file
    kind: http
    request:
      method: POST
      path: /api/process
      body:
        file: test-document.pdf
    expect:
      status_code: 200

  - name: verify processed file
    kind: minio
    verify_state:
      files_exist:
        - processed/test-document.pdf
      bucket_counts:
        processed: 1
      forbidden:
        buckets:
          uploads:
            - "*.tmp"
            - "temp_*"
```

---

## Multi-Service Testing

### Microservices Architecture

Testing multiple services that communicate with each other.

```yaml
kind: e2e_test:v1
name: microservices

fixtures:
  - correlation_id: test-12345

units:
  # Database
  - name: database
    kind: mongo
    image: mongo:6.0
    app_port: 27017
    migrations: db.js
  
  # Object Storage
  - name: storage
    kind: minio
    access_key: testkey
    secret_key: testsecret
    app_port: 9000
    buckets:
      - user-uploads
  
  # External API Mock
  - name: payment-api
    kind: httpmock
    app_port: 8081
    routes:
      - path: /api/charge
        method: POST
        response:
          status: 200
          body:
            transaction_id: "txn_123"
            status: "success"
  
  # User Service
  - name: user-service
    kind: http
    dockerfile: ./services/user/Dockerfile
    app_port: 8082
    healthcheck: /health
    env:
      - DATABASE_URL={{ database.dsn }}
  
  # Main API Gateway
  - name: api-gateway
    kind: http
    dockerfile: ./services/gateway/Dockerfile
    app_port: 8080
    healthcheck: /health
    env:
      - USER_SERVICE_URL=http://{{ user-service.host }}:{{ user-service.port }}
      - PAYMENT_SERVICE_URL=http://{{ payment-api.host }}:{{ payment-api.port }}
      - STORAGE_ENDPOINT={{ storage.local_endpoint }}
      - STORAGE_ACCESS_KEY={{ storage.access_key }}
      - STORAGE_SECRET_KEY={{ storage.secret_key }}

target: api-gateway

tests:
  - name: create user workflow
    kind: http
    request:
      method: POST
      path: /api/users
      headers:
        X-Correlation-ID: "{{ correlation_id }}"
      body:
        name: "John Doe"
        email: "john@example.com"
    expect:
      status_code: 201
      body_asserts:
        id:
          present: true
      header_asserts:
        X-Correlation-ID: "{{ correlation_id }}"

  - name: process payment
    kind: http
    request:
      method: POST
      path: /api/payments
      body:
        user_id: "user123"
        amount: 99.99
    expect:
      status_code: 200
      body_asserts:
        transaction_id: "txn_123"
        status: "success"
```

---

## Advanced Patterns

### Retry and Error Handling

```yaml
kind: e2e_test:v1
name: error-handling

units:
  - name: api
    kind: httpmock
    app_port: 8080
    routes:
      - path: /api/flaky
        method: GET
        response:
          status: 503
          body:
            error: "Service temporarily unavailable"
      
      - path: /api/error
        method: GET
        response:
          status: 500
          body:
            error: "Internal server error"
            code: "ERR_INTERNAL"

target: api

tests:
  - name: handle service unavailable
    kind: http
    request:
      path: /api/flaky
      method: GET
    expect:
      status_code: 503
      body_asserts:
        error:
          contains: "unavailable"

  - name: handle internal error
    kind: http
    request:
      path: /api/error
      method: GET
    expect:
      status_code: 500
      body_asserts:
        code: "ERR_INTERNAL"
```

### Complex Assertions

```yaml
kind: e2e_test:v1
name: complex-assertions

units:
  - name: api
    kind: httpmock
    app_port: 8080
    routes:
      - path: /api/data
        method: GET
        response:
          status: 200
          body:
            users:
              - id: 1
                name: "Alice"
                age: 30
                tags: ["admin", "user"]
              - id: 2
                name: "Bob"
                age: 25
                tags: ["user"]
            metadata:
              total: 2
              page: 1
              timestamp: "2024-01-15T10:30:00Z"

target: api

tests:
  - name: complex response validation
    kind: http
    request:
      path: /api/data
      method: GET
    expect:
      status_code: 200
      body_asserts:
        # Array validation
        users:
          type: array
          length: 2
        
        # Element access
        users.0.name: "Alice"
        users.0.age:
          ">": 18
          "<": 100
        users.0.tags:
          type: array
        
        # Array containment - check if array contains item matching conditions
        users:
          contains_where:
            name: "Alice"
            age:
              ">": 18
        
        # Check all items match conditions
        users:
          all_match:
            active: true
        
        # Metadata validation
        total: 2
        metadata.total: 2
        
        metadata.timestamp:
          present: true
          matches: "^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$"
```

### Environment-Specific Configuration

```yaml
kind: e2e_test:v1
name: env-specific

fixtures:
  - env: test
  - api_host: localhost
  - log_level: debug

units:
  - name: app
    kind: http
    dockerfile: Dockerfile
    app_port: 8080
    env_file: .env.test
    env:
      - ENV={{ env }}
      - LOG_LEVEL={{ log_level }}
      - API_HOST={{ api_host }}
      - FEATURE_FLAG_NEW_UI=true

target: app

tests:
  - name: verify environment
    kind: http
    request:
      path: /api/config
      method: GET
    expect:
      status_code: 200
      body_asserts:
        environment: "{{ env }}"
        features.new_ui: true
```

### Load Testing Pattern

```yaml
kind: e2e_test:v1
name: load-test

units:
  - name: api
    kind: http
    dockerfile: Dockerfile
    app_port: 8080

target: api

tests:
  # Warm-up
  - name: warmup request 1
    kind: http
    request:
      path: /api/ping
      method: GET
    expect:
      status_code: 200

  # Repeated requests to simulate load
  - name: load test 1
    kind: http
    request:
      path: /api/users
      method: GET
    expect:
      status_code: 200

  - name: load test 2
    kind: http
    request:
      path: /api/users
      method: GET
    expect:
      status_code: 200

  - name: load test 3
    kind: http
    request:
      path: /api/users
      method: GET
    expect:
      status_code: 200

  # Verify system health after load
  - name: health check after load
    kind: http
    request:
      path: /health
      method: GET
    expect:
      status_code: 200
      body_asserts:
        status: "healthy"

## Array Assertions

### Testing Array Contents with Complex Objects

```yaml
kind: e2e_test:v1
name: array-testing

units:
  - name: api
    kind: httpmock
    app_port: 8080
    routes:
      - path: /products
        method: GET
        response:
          status: 200
          body:
            products:
              - id: 1
                name: "iPhone"
                price: 999
                category: "electronics"
                in_stock: true
              - id: 2
                name: "iPad"
                price: 799
                category: "electronics"
                in_stock: true
              - id: 3
                name: "MacBook"
                price: 1999
                category: "electronics"
                in_stock: false

target: api

tests:
  - name: test array contains specific item
    kind: http
    request:
      path: /products
      method: GET
    expect:
      status_code: 200
      body_asserts:
        # Check if array contains item with specific name
        products:
          contains_where:
            name: "iPhone"
  
  - name: test array contains item matching multiple conditions
    kind: http
    request:
      path: /products
      method: GET
    expect:
      status_code: 200
      body_asserts:
        # Check if array contains expensive out-of-stock item
        products:
          contains_where:
            category: "electronics"
            price:
              ">": 1500
            in_stock: false
  
  - name: test all items match conditions
    kind: http
    request:
      path: /products
      method: GET
    expect:
      status_code: 200
      body_asserts:
        # Ensure all products are electronics with positive price
        products:
          all_match:
            category: "electronics"
            price:
              ">": 0
  
  - name: test no items match condition
    kind: http
    request:
      path: /products
      method: GET
    expect:
      status_code: 200
      body_asserts:
        # Ensure no products are overpriced
        products:
          none_match:
            price:
              ">": 5000
```

**Key Features:**
- `contains_where`: Checks if at least one array element matches all specified conditions
- `all_match`: Ensures every array element matches the conditions
- `none_match`: Ensures no array elements match the conditions
- Works with nested conditions like `>`, `<`, `equals`, `contains`, etc.
- Can combine multiple conditions on different fields
```

---

## Resource Management & Cleanup

ENE automatically manages Docker resources during test execution. However, understanding cleanup is important for CI/CD pipelines and troubleshooting.

### Automatic Cleanup in Tests

ENE uses a `CleanupRegistry` that ensures proper cleanup order automatically:

```go
// This happens automatically in TestSuiteV1
registry := e2eframe.NewCleanupRegistry()
defer registry.CleanupAll(context.Background())

// Resources are registered as they're created
registry.Register(e2eframe.NewCleanableNetwork(network))
registry.Register(e2eframe.NewCleanableContainer(container, "unit-name"))

// Cleanup happens in correct order:
// 1. Containers (first)
// 2. Networks (after containers detach)
// 3. Other resources
```

**Key Benefits:**
- No "network has active endpoints" errors
- Resources cleaned even if tests fail
- Proper error handling and reporting

### Manual Cleanup Command

The `ene cleanup` command removes orphaned Docker resources.

**Basic Usage:**

```bash
# Interactive cleanup (asks for confirmation)
ene cleanup

# Preview what would be removed (safe)
ene cleanup --dry-run --verbose

# Force cleanup without confirmation
ene cleanup --force

# Clean specific resource types
ene cleanup networks --force
ene cleanup containers --force
```

**Age-Based Filtering:**

```bash
# Clean resources older than 1 hour
ene cleanup --older-than=1h --force

# Clean resources older than 24 hours
ene cleanup --older-than=24h --force

# Clean resources older than 30 minutes
ene cleanup --older-than=30m --force
```

**Advanced Options:**

```bash
# Include all resources (not just orphaned)
ene cleanup --all --force

# Verbose output for debugging
ene cleanup --dry-run --verbose

# Combine options
ene cleanup --older-than=2h --verbose --force
```

### CI/CD Integration Examples

**GitLab CI:**

```yaml
# .gitlab-ci.yml
test:
  image: golang:1.25
  services:
    - docker:dind
  before_script:
    - apt-get update && apt-get install -y docker.io
    - go build -o ene .
  script:
    - ./ene --verbose
  after_script:
    # Always clean up, even if tests fail
    - ./ene cleanup --older-than=30m --force
  artifacts:
    when: always
    reports:
      junit: report.xml
```

**GitHub Actions:**

```yaml
# .github/workflows/test.yml
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
          go-version: '1.25'
      
      - name: Build ENE
        run: go build -o ene .
      
      - name: Run Tests
        run: ./ene --verbose --html=report.html
      
      - name: Cleanup Docker Resources
        if: always()
        run: ./ene cleanup --older-than=30m --force --verbose
      
      - name: Upload Test Report
        if: always()
        uses: actions/upload-artifact@v3
        with:
          name: test-report
          path: report.html
```

**Jenkins Pipeline:**

```groovy
// Jenkinsfile
pipeline {
    agent any
    
    stages {
        stage('Build') {
            steps {
                sh 'go build -o ene .'
            }
        }
        
        stage('E2E Tests') {
            steps {
                sh './ene --verbose --json=report.json'
            }
        }
    }
    
    post {
        always {
            // Cleanup regardless of test results
            sh './ene cleanup --older-than=1h --force'
            
            // Archive reports
            archiveArtifacts artifacts: 'report.json', allowEmptyArchive: true
        }
    }
}
```

**CircleCI:**

```yaml
# .circleci/config.yml
version: 2.1

jobs:
  test:
    docker:
      - image: cimg/go:1.25
    steps:
      - checkout
      - setup_remote_docker
      
      - run:
          name: Build ENE
          command: go build -o ene .
      
      - run:
          name: Run E2E Tests
          command: ./ene --verbose --html=report.html
      
      - run:
          name: Cleanup Docker Resources
          command: ./ene cleanup --older-than=30m --force
          when: always
      
      - store_artifacts:
          path: report.html
          destination: test-report

workflows:
  test:
    jobs:
      - test
```

### Periodic Cleanup with Cron

For servers running ENE regularly, set up periodic cleanup:

```bash
# Edit crontab
crontab -e

# Add nightly cleanup (2 AM daily)
0 2 * * * /usr/local/bin/ene cleanup --older-than=24h --force >> /var/log/ene-cleanup.log 2>&1

# Or every 6 hours
0 */6 * * * /usr/local/bin/ene cleanup --older-than=12h --force >> /var/log/ene-cleanup.log 2>&1
```

### Troubleshooting Resource Leaks

**Scenario: Tests are leaving resources behind**

```bash
# Step 1: Check what's orphaned
ene cleanup --dry-run --verbose

# Output shows:
# Found 3 orphaned resources:
# 
# Containers (2):
#   - testcontainers-abc123 (postgres, stopped 2h ago)
#   - testcontainers-def456 (httpunit, stopped 3h ago)
# 
# Networks (1):
#   - testcontainers-xyz789 (created 2h ago, 0 containers)

# Step 2: Clean them up
ene cleanup --force
```

**Scenario: "Network has active endpoints" error**

```bash
# The new cleanup command handles this automatically
# But if you still see this error:

# Step 1: Clean containers first
ene cleanup containers --force

# Step 2: Then clean networks
ene cleanup networks --force

# Or let the command handle ordering automatically
ene cleanup --force
```

**Scenario: Manual Docker inspection**

```bash
# List all testcontainers networks
docker network ls | grep testcontainers

# List all testcontainers containers
docker ps -a | grep testcontainers

# Inspect a specific network
docker network inspect <network-id>

# Remove manually if needed
docker rm -f <container-id>
docker network rm <network-id>
```

### Best Practices

**For Local Development:**

```bash
# Check for orphaned resources after testing session
ene cleanup --dry-run --verbose

# Clean up when done
ene cleanup --force
```

**For CI/CD:**

```yaml
# Always clean up in after_script/post steps
after_script:
  - ene cleanup --older-than=30m --force
```

**For Shared Test Environments:**

```bash
# Be more conservative with age filtering
ene cleanup --older-than=2h --force

# Or use dry-run first to verify
ene cleanup --older-than=2h --dry-run --verbose
```

**For Production CI Servers:**

```bash
# Daily cleanup of old resources
0 2 * * * /usr/local/bin/ene cleanup --older-than=24h --force

# Monitoring: Alert if cleanup fails
0 2 * * * /usr/local/bin/ene cleanup --older-than=24h --force || /usr/local/bin/alert-ops "ENE cleanup failed"
```

### Migration from Old Cleanup

If you were using the old `cleanup-networks` command:

```bash
# Old (deprecated)
ene cleanup-networks --all

# New (recommended)
ene cleanup --force

# With age filtering (new capability)
ene cleanup --older-than=1h --force
```

For more details, see:
- [Cleanup Architecture Guide](../e2eframe/CLEANUP_ARCHITECTURE.md)
- [Migration Guide](../e2eframe/MIGRATION_GUIDE.md)
- [CLI Usage - Cleanup Command](CLI_USAGE.md#cleanup)

---

## Running Examples

```bash
# Run all examples in parallel
ene --parallel

# Run specific example with verbose output
ene --suite=mongo-integration --verbose

# Run with debugging
ene --suite=microservices --debug --verbose

# Generate reports
ene --suite=storage-test --html=storage-report.html

# Validate before running
ene dry-run tests/user-crud/suite.yml --verbose
```

## Tips for Writing Tests

1. **Keep tests independent** - Each test should be able to run in isolation
2. **Use descriptive names** - Test names should clearly describe what is being tested
3. **Leverage fixtures** - Reuse common values across tests
4. **Set appropriate timeouts** - Balance speed and reliability
5. **Use healthchecks** - Ensure services are ready before testing
6. **Clean test data** - Use migrations to set up known state
7. **Test error cases** - Don't just test happy paths
8. **Use meaningful assertions** - Test what matters for your application
9. **Read error messages carefully** - ENE provides detailed error messages showing both expected and actual values to help diagnose failures quickly

### Understanding Test Failures

When assertions fail, ENE provides detailed error messages that show both what was expected and what was actually received:

**Header Assertion Failures:**
```
✗ header "Content-Type": expected "application/json" but got "text/plain"
✗ header "Cache-Control" does not contain "no-cache" (got: "public, max-age=3600")
```

**Body Assertion Failures:**
```
✗ expected "John Doe" but got "Jane Smith"
✗ expected value > 18 but got 15
✗ expected array size 5 but got 3
✗ expected type 'string' but got type 'number' at path: user.age (value: "25")
```

These messages help you quickly identify:
- What value was expected
- What value was actually received
- Where in the response the mismatch occurred (for body assertions)

**Example Test Failure Output:**
```
[1/1] api-tests (Setup: 50ms | Tests: 0ms | Overhead: 6.00s)
  ✗  create user (failed after 3 retries)
     └─ header "Content-Type": expected "application/json; charset=utf-8" but got "application/json"
```

This makes debugging much faster as you can immediately see the difference between expected and actual values without needing to inspect raw responses.

---

For more information, see:
- [CLI Usage Guide](CLI_USAGE.md)
- [Configuration Reference](CONFIGURATION_REFERENCE.md)
- [Quick Reference](QUICK_REFERENCE.md)