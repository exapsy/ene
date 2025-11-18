# ENE Examples and Recipes

Practical examples and recipes for common testing scenarios with ENE.

## Table of Contents

- [Basic Examples](#basic-examples)
- [API Testing](#api-testing)
- [Database Testing](#database-testing)
- [Object Storage Testing](#object-storage-testing)
- [Multi-Service Testing](#multi-service-testing)
- [Advanced Patterns](#advanced-patterns)

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
        - path: message
          equals: "Hello, World!"
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
  - name: api_version
    value: v1
  - name: user_id
    value: "12345"

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
        - path: id
          present: true
        - path: name
          equals: "John Doe"
      header_asserts:
        - name: Location
          contains: /users/

  - name: get user
    kind: http
    request:
      method: GET
      path: /v1/users/{{ user_id }}
    expect:
      status_code: 200
      body_asserts:
        - path: id
          equals: "{{ user_id }}"
        - path: email
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
        - path: name
          equals: "Jane Doe"

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
  - name: username
    value: testuser
  - name: password
    value: testpass123
  - name: auth_token
    value: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9

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
        - path: token
          present: true
          type: string
        - path: expires_in
          greater_than: 0

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
        - path: username
          equals: "{{ username }}"

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
        - path: database.connected
          equals: true
        - path: database.name
          equals: testdb

  - name: list users from seed data
    kind: http
    request:
      path: /api/users
      method: GET
    expect:
      status_code: 200
      body_asserts:
        - path: users
          type: array
        - path: users
          size: 2
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
        - path: products
          type: array
        - path: total
          greater_than: 0
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
        - path: uploaded
          equals: true
        - path: file_id
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
  - name: correlation_id
    value: test-12345

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
        - path: id
          present: true
      header_asserts:
        - name: X-Correlation-ID
          equals: "{{ correlation_id }}"

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
        - path: transaction_id
          equals: "txn_123"
        - path: status
          equals: "success"
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
        - path: error
          contains: "unavailable"

  - name: handle internal error
    kind: http
    request:
      path: /api/error
      method: GET
    expect:
      status_code: 500
      body_asserts:
        - path: code
          equals: "ERR_INTERNAL"
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
        - path: users
          type: array
          size: 2
        
        # Nested field validation
        - path: users[0].name
          equals: "Alice"
        
        - path: users[0].age
          greater_than: 18
          less_than: 100
        
        - path: users[0].tags
          type: array
        
        # Metadata validation
        - path: metadata.total
          equals: 2
        
        - path: metadata.timestamp
          present: true
          matches: "^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$"
```

### Environment-Specific Configuration

```yaml
kind: e2e_test:v1
name: env-specific

fixtures:
  - name: env
    value: test
  - name: api_host
    value: localhost
  - name: log_level
    value: debug

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
        - path: environment
          equals: "{{ env }}"
        - path: features.new_ui
          equals: true
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
        - path: status
          equals: "healthy"
```

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

---

For more information, see:
- [CLI Usage Guide](CLI_USAGE.md)
- [Configuration Reference](CONFIGURATION_REFERENCE.md)
- [Quick Reference](QUICK_REFERENCE.md)