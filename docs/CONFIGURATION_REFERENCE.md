# ENE Test Configuration Reference

Complete reference for ENE test suite configuration files (`suite.yml`).

## Table of Contents

- [Configuration Format](#configuration-format)
- [Top-Level Fields](#top-level-fields)
- [Fixtures](#fixtures)
- [Units](#units)
- [Tests](#tests)
- [Assertions](#assertions)
- [Variable Interpolation](#variable-interpolation)

---

## Configuration Format

ENE uses YAML format for test configuration. Each test suite must have a `suite.yml` file.

### Basic Structure

```yaml
kind: e2e_test:v1
name: my-test-suite

fixtures:
  - name: fixture_name
    value: fixture_value

units:
  - name: service_name
    kind: service_type
    # ... service configuration

target: target_service_name

tests:
  - name: test_name
    kind: test_type
    # ... test configuration
```

---

## Top-Level Fields

### `kind` (required)

Specifies the configuration version.

- **Type**: `string`
- **Required**: Yes
- **Valid Values**: `e2e_test:v1`

```yaml
kind: e2e_test:v1
```

### `name` (required)

Name of the test suite. Should be descriptive and unique.

- **Type**: `string`
- **Required**: Yes
- **Min Length**: 1

```yaml
name: user-api-tests
```

### `fixtures` (optional)

Array of reusable data fixtures that can be interpolated throughout the configuration.

- **Type**: `array`
- **Required**: No
- **Default**: `[]`

See [Fixtures](#fixtures) section for details.

### `units` (required)

Array of service/container definitions that make up the test environment.

- **Type**: `array`
- **Required**: Yes
- **Min Items**: 1

See [Units](#units) section for details.

### `target` (required)

Name of the unit that tests should target by default.

- **Type**: `string`
- **Required**: Yes
- **Must Reference**: An existing unit name

```yaml
target: my-app
```

### `tests` (required)

Array of test cases to execute.

- **Type**: `array`
- **Required**: Yes
- **Min Items**: 1

See [Tests](#tests) section for details.

---

## Fixtures

Fixtures are reusable values that can be referenced throughout the configuration using `{{ fixture_name }}` syntax.

### Inline Fixture

```yaml
fixtures:
  - name: api_key
    value: test-key-12345
```

**Fields:**
- `name` (required): Unique identifier for the fixture
- `value` (required): The value to use (string, number, boolean, or object)

### File-Based Fixture

```yaml
fixtures:
  - name: test_user
    file: ./testdata/user.json
```

**Fields:**
- `name` (required): Unique identifier for the fixture
- `file` (required): Path to file containing the fixture value (relative to suite directory)

### Usage Example

```yaml
fixtures:
  - name: content_type
    value: application/json; charset=utf-8
  - name: test_payload
    file: ./data/payload.json

tests:
  - name: create resource
    kind: http
    request:
      headers:
        Content-Type: "{{ content_type }}"
      body: "{{ test_payload }}"
```

---

## Units

Units define the services/containers that make up your test environment.

### Common Fields (All Unit Types)

```yaml
units:
  - name: my-service
    kind: http
    app_port: 8080
    startup_timeout: 30s
    env_file: .env
    env:
      - KEY=value
```

**Common Fields:**
- `name` (required): Unique identifier for this unit
- `kind` (required): Type of unit (see unit types below)
- `app_port` (required): Port the service listens on
- `startup_timeout` (optional): Maximum time to wait for startup (default: 30s)
- `env_file` (optional): Path to environment file (relative to suite directory)
- `env` (optional): Array of environment variables in `KEY=value` format

### Unit Type: `httpmock`

Mock HTTP server for testing without real services.

```yaml
- name: mock-api
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
    - path: /api/users
      method: POST
      response:
        status: 201
        body:
          id: "123"
          created: true
```

**Fields:**
- `routes` (required): Array of mock route definitions
  - `path` (required): URL path to match
  - `method` (required): HTTP method (GET, POST, PUT, DELETE, PATCH, etc.)
  - `response` (required): Response configuration
    - `status` (required): HTTP status code
    - `body` (optional): Response body (can be object or string)
    - `headers` (optional): Response headers (key-value pairs)

**Supported Methods**: GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS

### Unit Type: `http`

Your actual application service running in a container.

```yaml
- name: my-app
  kind: http
  dockerfile: Dockerfile
  # OR
  image: myapp:latest
  app_port: 8080
  healthcheck: /health
  startup_timeout: 2m
  env_file: .env
  env:
    - DATABASE_URL={{ mongodb.dsn }}
    - LOG_LEVEL=debug
  cmd:
    - ./server
    - --port=8080
```

**Fields:**
- `dockerfile` (conditional): Path to Dockerfile (required if `image` not specified)
- `image` (conditional): Docker image name (required if `dockerfile` not specified)
- `healthcheck` (optional): Health check endpoint path
- `cmd` (optional): Override container command (array of strings)

**Note**: Either `dockerfile` or `image` must be specified, but not both.

### Unit Type: `mongo`

MongoDB database service.

```yaml
- name: mongodb
  kind: mongo
  image: mongo:6.0
  app_port: 27017
  database: testdb
  user: testuser
  password: testpass
  migrations: db.js
  startup_timeout: 30s
```

**Fields:**
- `image` (required): MongoDB Docker image
- `database` (optional): Database name (default: test)
- `user` (optional): Database user (default: admin)
- `password` (optional): Database password (default: password)
- `migrations` (optional): Path to JavaScript migration file

**Available Service Variables:**
- `{{ mongodb.dsn }}` - Full connection string
- `{{ mongodb.host }}` - Hostname
- `{{ mongodb.port }}` - Port number
- `{{ mongodb.database }}` - Database name

### Unit Type: `postgres`

PostgreSQL database service.

```yaml
- name: postgres
  kind: postgres
  image: postgres:14
  app_port: 5432
  database: testdb
  user: testuser
  password: testpass
  migrations: ./migrations
  startup_timeout: 30s
```

**Fields:**
- `image` (required): PostgreSQL Docker image
- `database` (optional): Database name (default: test)
- `user` (optional): Database user (default: postgres)
- `password` (optional): Database password (default: postgres)
- `migrations` (optional): Path to SQL migration files directory

**Available Service Variables:**
- `{{ postgres.dsn }}` - Full connection string
- `{{ postgres.host }}` - Hostname
- `{{ postgres.port }}` - Port number
- `{{ postgres.database }}` - Database name

### Unit Type: `minio`

MinIO S3-compatible object storage service.

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
    - archived
  env:
    - MINIO_BROWSER=on
```

**Fields:**
- `image` (required): MinIO Docker image
- `access_key` (required): Access key for MinIO
- `secret_key` (required): Secret key for MinIO
- `console_port` (optional): MinIO console port (default: 9001)
- `buckets` (optional): Array of bucket names to create on startup

**Available Service Variables:**
- `{{ storage.endpoint }}` - External endpoint
- `{{ storage.local_endpoint }}` - Internal container endpoint
- `{{ storage.access_key }}` - Access key
- `{{ storage.secret_key }}` - Secret key

---

## Tests

Test definitions specify what to test and what results to expect.

### Common Test Fields

```yaml
tests:
  - name: my test
    kind: http
    timeout: 5s
```

**Common Fields:**
- `name` (required): Descriptive test name
- `kind` (required): Test type (`http` or `minio`)
- `timeout` (optional): Maximum test execution time (default: 5s)

### Test Type: `http`

HTTP request/response test.

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

#### `request` Object

**Fields:**
- `method` (required): HTTP method (GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS)
- `path` (required): Request path (can include variables)
- `timeout` (optional): Request timeout (default: 5s)
- `headers` (optional): Request headers (key-value pairs)
- `query_params` (optional): Query parameters (key-value pairs)
- `body` (optional): Request body (can be object or string)

#### `expect` Object

**Fields:**
- `status_code` (optional): Expected HTTP status code (default: 200)
- `body_asserts` (optional): Array of body assertions (see [Body Assertions](#body-assertions))
- `header_asserts` (optional): Array of header assertions (see [Header Assertions](#header-assertions))

### Test Type: `minio`

MinIO state verification test.

```yaml
- name: verify upload
  kind: minio
  verify_state:
    files_exist:
      - uploads/file1.txt
      - uploads/file2.pdf
    bucket_counts:
      uploads: 2
      processed: 0
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
          - "temp_*"
    constraints:
      - bucket: uploads
        file_count: 2
        max_total_size: 100MB
```

#### `verify_state` Object

**Fields:**
- `files_exist` (optional): Array of file paths that must exist
- `bucket_counts` (optional): Expected file counts per bucket (key-value pairs)
- `required` (optional): Required file specifications
- `forbidden` (optional): Patterns/files that must not exist
- `constraints` (optional): Bucket-level constraints

#### `required.buckets` Specifications

```yaml
required:
  buckets:
    bucket_name:
      - path: relative/path/to/file
        min_size: 1B
        max_size: 10MB
        content_type: image/jpeg
        max_age: 5m
        pattern: "^user-.*\\.jpg$"
```

**Fields:**
- `path` (required): Relative path to file in bucket
- `min_size` (optional): Minimum file size (e.g., "1B", "1KB", "1MB")
- `max_size` (optional): Maximum file size
- `max_age` (optional): Maximum file age (e.g., "5m", "1h", "2d")
- `content_type` (optional): Expected MIME type
- `pattern` (optional): Regex pattern for file path

#### `forbidden` Specifications

```yaml
forbidden:
  buckets:
    bucket_name:
      - "*.tmp"           # Wildcard patterns
      - "temp_*"
      - "debug/**"
  files:
    - "uploads/sensitive.txt"
    - "bucket/admin/*"
```

#### `constraints` Specifications

```yaml
constraints:
  - bucket: uploads
    file_count: 5                    # Exact count
    # OR
    file_count: ">= 5"              # Comparison
    max_total_size: 100MB
    min_total_size: 1MB
  
  - bucket: archived
    file_count: 0                    # Must be empty
  
  - total_buckets: 3                 # Total bucket count
  - empty_buckets: "<= 1"            # Empty bucket limit
```

**Constraint Types:**
- `bucket` (optional): Specific bucket name
- `file_count` (optional): Expected file count (number or comparison string)
- `max_total_size` (optional): Maximum total size of files
- `min_total_size` (optional): Minimum total size of files
- `total_buckets` (optional): Total number of buckets
- `empty_buckets` (optional): Number of empty buckets allowed

**Comparison Operators**: `=`, `!=`, `>`, `<`, `>=`, `<=`, `≤`, `≥`

---

## Assertions

### Body Assertions

Assertions on response body content using JSONPath. Uses a map-based format where keys are JSON paths and values are either strings (shorthand for equals) or objects with explicit assertions.

```yaml
body_asserts:
  # String shorthand for equals
  user.name: John Doe
  
  # Explicit assertions with object
  user.email:
    matches: "^[a-z0-9._%+-]+@[a-z0-9.-]+\\.[a-z]{2,}$"
  
  # Comparison operators with symbols
  user.age:
    ">": 18        # greater_than
    "<": 100       # less_than
  
  # Type and length checking
  user.tags:
    length: 3      # or 'size'
    type: array
  
  # Multiple assertions on same field
  user.active:
    type: boolean
    equals: true
```

#### Assertion Fields

**Map Key:** JSONPath to the field (use `$` for root document)

**Map Value:** Either a string (shorthand for `equals`) or an object with one or more assertions:

**Comparison Assertions:**
- String value: Shorthand for equals assertion (e.g., `name: "John"`)
- `equals`: Exact value match
- `not_equals`: Value must not match
- `contains`: String/array must contain substring/element
- `not_contains`: Must not contain substring/element
- `matches`: Regex pattern match
- `not_matches`: Must not match regex pattern
- `>`: Numeric comparison (value > threshold)
- `<`: Numeric comparison (value < threshold)
- `greater_than`: Alias for `>`
- `less_than`: Alias for `<`

**Presence Assertion:**
- `present`: Boolean - field must exist (true) or not exist (false)

**Type Assertion:**
- `type`: Expected type (`string`, `int`, `float`, `bool`, `array`, `object`)

**Size/Length Assertion:**
- `length`: Expected length for strings/arrays or key count for objects
- `size`: Alias for `length`

**Note:** Some assertions conflict (e.g., `equals` with `contains`, `>`, `<`). Compatible combinations include `>` and `<` for range checks, `present` with any other assertion, and `type` with any other assertion.

#### JSONPath Examples

```yaml
body_asserts:
  # Root document
  $:
    type: object
  
  # Top-level field with shorthand
  status: success
  
  # Nested field
  data.user.email:
    present: true
  
  # Array element with shorthand
  items.0.name: First Item
  
  # Array length
  items:
    length: 5
  
  # Range checking
  score:
    ">": 0
    "<": 100
```

### Header Assertions

Assertions on response headers. Uses a map-based format where keys are header names and values are either strings (shorthand for equals) or objects with explicit assertions.

```yaml
header_asserts:
  # String shorthand for equals
  Content-Type: application/json
  
  # Explicit assertions with object
  X-Request-ID:
    present: true
    matches: "^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"
  
  # String operations
  Cache-Control:
    contains: no-cache
  
  Set-Cookie:
    not_contains: secure=false
```

#### Assertion Fields

**Map Key:** HTTP header name (case-insensitive)

**Map Value:** Either a string (shorthand for `equals`) or an object with one or more assertions:

**Comparison Assertions:**
- String value: Shorthand for equals assertion (e.g., `Content-Type: application/json`)
- `equals`: Exact value match
- `not_equals`: Value must not match
- `contains`: Header value must contain substring
- `not_contains`: Must not contain substring
- `matches`: Regex pattern match
- `not_matches`: Must not match regex pattern

**Presence Assertion:**
- `present`: Boolean - header must exist (true) or not exist (false)

**Note:** `equals` conflicts with other comparison assertions. Compatible combinations include `present` with any other assertion, `contains` with `matches`, etc.

---

## Variable Interpolation

ENE supports variable interpolation using `{{ variable_name }}` syntax.

### Fixture Interpolation

Reference fixtures defined in the `fixtures` section.

```yaml
fixtures:
  - name: api_key
    value: test-key-123

tests:
  - name: test
    kind: http
    request:
      headers:
        Authorization: "Bearer {{ api_key }}"
```

### Service Variable Interpolation

Reference properties of defined units using `{{ unit_name.property }}` syntax.

#### MongoDB Variables

```yaml
- name: mongodb
  kind: mongo

# Available variables:
# {{ mongodb.dsn }}       - mongodb://user:pass@host:port/database
# {{ mongodb.host }}      - container hostname
# {{ mongodb.port }}      - 27017
# {{ mongodb.database }}  - database name
# {{ mongodb.user }}      - username
# {{ mongodb.password }}  - password
```

#### PostgreSQL Variables

```yaml
- name: postgres
  kind: postgres

# Available variables:
# {{ postgres.dsn }}      - postgres://user:pass@host:port/database
# {{ postgres.host }}     - container hostname
# {{ postgres.port }}     - 5432
# {{ postgres.database }} - database name
# {{ postgres.user }}     - username
# {{ postgres.password }} - password
```

#### MinIO Variables

```yaml
- name: storage
  kind: minio

# Available variables:
# {{ storage.endpoint }}        - http://host:9000 (external)
# {{ storage.local_endpoint }}  - host:9000 (internal)
# {{ storage.access_key }}      - access key
# {{ storage.secret_key }}      - secret key
# {{ storage.console_port }}    - 9001
```

#### HTTP Service Variables

```yaml
- name: my-app
  kind: http

# Available variables:
# {{ my-app.host }}     - container hostname
# {{ my-app.port }}     - service port
# {{ my-app.endpoint }} - http://host:port
```

### Interpolation Examples

```yaml
fixtures:
  - name: api_version
    value: v1

units:
  - name: database
    kind: mongo
  
  - name: app
    kind: http
    env:
      - DATABASE_URL={{ database.dsn }}
      - API_VERSION={{ api_version }}

tests:
  - name: test
    kind: http
    request:
      path: /api/{{ api_version }}/users
      headers:
        X-Database: "{{ database.host }}"
```

---

## Size Units

Size values support the following units:

- `B` - Bytes
- `KB` - Kilobytes (1024 bytes)
- `MB` - Megabytes (1024 KB)
- `GB` - Gigabytes (1024 MB)

Examples: `100B`, `1.5KB`, `10MB`, `2GB`

---

## Duration Format

Duration values use Go's duration format:

- `s` - Seconds
- `m` - Minutes
- `h` - Hours
- `d` - Days (custom extension)

Examples: `5s`, `2m`, `1h30m`, `2d`

---

## Complete Example

```yaml
kind: e2e_test:v1
name: complete-example

fixtures:
  - name: content_type
    value: application/json; charset=utf-8
  - name: test_user
    file: ./testdata/user.json

units:
  - name: mongodb
    kind: mongo
    image: mongo:6.0
    app_port: 27017
    database: testdb
    migrations: db.js
    startup_timeout: 30s
  
  - name: storage
    kind: minio
    image: minio/minio:latest
    access_key: testkey
    secret_key: testsecret
    app_port: 9000
    console_port: 9001
    buckets:
      - uploads
  
  - name: app
    kind: http
    dockerfile: Dockerfile
    app_port: 8080
    healthcheck: /health
    startup_timeout: 2m
    env:
      - DATABASE_URL={{ mongodb.dsn }}
      - STORAGE_ENDPOINT={{ storage.local_endpoint }}
      - STORAGE_ACCESS_KEY={{ storage.access_key }}
      - STORAGE_SECRET_KEY={{ storage.secret_key }}

target: app

tests:
  - name: health check
    kind: http
    request:
      path: /health
      method: GET
    expect:
      status_code: 200
      body_asserts:
        status: ok
  
  - name: create user
    kind: http
    request:
      path: /api/users
      method: POST
      headers:
        Content-Type: "{{ content_type }}"
      body: "{{ test_user }}"
    expect:
      status_code: 201
      body_asserts:
        id:
          present: true
          type: string
        created: true
      header_asserts:
        Location:
          present: true
          matches: "^/api/users/[0-9]+$"
  
  - name: upload file
    kind: http
    request:
      path: /api/upload
      method: POST
      headers:
        Content-Type: multipart/form-data
      body:
        file: testfile.txt
    expect:
      status_code: 200
  
  - name: verify file in storage
    kind: minio
    verify_state:
      files_exist:
        - uploads/testfile.txt
      bucket_counts:
        uploads: 1
      required:
        buckets:
          uploads:
            - path: testfile.txt
              min_size: 1B
              max_size: 10MB
              max_age: 1m
```

---

## Validation

ENE validates configuration against a JSON schema during load. Common validation errors:

1. **Missing Required Fields**: Ensure all required fields are present
2. **Invalid Types**: Check field types match specifications
3. **Invalid References**: Ensure fixture/unit names exist before referencing
4. **Invalid Syntax**: Verify YAML syntax is correct

Run `ene dry-run` to validate configuration without running tests.

---

## Best Practices

1. **Use Descriptive Names**: Make test and unit names clear and descriptive
2. **Organize Fixtures**: Group related fixtures together
3. **Minimize Timeouts**: Set appropriate timeouts to avoid slow tests
4. **Use Healthchecks**: Define healthcheck endpoints for faster startup detection
5. **Leverage Interpolation**: Use fixtures and service variables to avoid duplication
6. **Test Independently**: Design tests to run independently without dependencies
7. **Clean Test Data**: Use migrations to set up known test data state
8. **Document Tests**: Use clear test names that describe what is being tested

---

## Schema Version

Current schema version: `e2e_test:v1`

For schema updates and migration guides, see the CHANGELOG.