# PostgreSQL Test Plugin

This plugin provides PostgreSQL database testing capabilities for the ene e2e testing framework.

## Overview

The `postgrestest` plugin allows you to:

- Execute SQL queries against PostgreSQL databases
- Assert row counts (exact, min/max ranges, or zero)
- Verify exact row data
- Check table existence
- Assert specific column values
- Test if results contain or don't contain specific rows
- Validate data integrity and relationships

## Installation

This plugin is automatically registered when you import it in your main application:

```go
import _ "github.com/exapsy/ene/plugins/postgrestest"
```

## Usage

### Basic Example

```yaml
kind: e2e_test:v1
name: my-test

units:
  - kind: postgres
    name: testdb
    image: postgres:15-alpine
    app_port: 5432
    database: testdb
    user: testuser
    password: testpass

target: testdb

tests:
  - name: "Check user count"
    kind: postgres
    query: "SELECT COUNT(*) as count FROM users"
    expect:
      row_count: 1
      column_values:
        count: 5
```

## Configuration

### Test Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique test identifier |
| `kind` | string | Yes | Must be `"postgres"` |
| `query` | string | Conditional | SQL query to execute (required unless only checking `table_exists`) |
| `expect` | object | Yes | Expectations to verify |

### Expectation Options

| Field | Type | Description |
|-------|------|-------------|
| `table_exists` | string | Verify a table exists |
| `row_count` | integer | Assert exact row count |
| `min_row_count` | integer | Assert minimum row count |
| `max_row_count` | integer | Assert maximum row count |
| `no_rows` | boolean | Assert query returns zero rows |
| `rows` | array[object] | Assert exact row data matches |
| `column_values` | object | Assert column values (single row only) |
| `contains` | array[object] | Assert results contain these rows |
| `not_contains` | array[object] | Assert results don't contain these rows |

## Examples

### Table Existence Check

```yaml
- name: "Verify users table exists"
  kind: postgres
  expect:
    table_exists: "users"
```

### Row Count Assertions

```yaml
# Exact count
- name: "Verify 5 users"
  kind: postgres
  query: "SELECT * FROM users"
  expect:
    row_count: 5

# Range
- name: "Verify user count in range"
  kind: postgres
  query: "SELECT * FROM users"
  expect:
    min_row_count: 1
    max_row_count: 100

# No rows
- name: "No orphaned records"
  kind: postgres
  query: "SELECT * FROM orders WHERE user_id NOT IN (SELECT id FROM users)"
  expect:
    no_rows: true
```

### Exact Row Data

```yaml
- name: "Verify user details"
  kind: postgres
  query: "SELECT id, name, email FROM users WHERE id = 1"
  expect:
    row_count: 1
    rows:
      - id: 1
        name: "Alice"
        email: "alice@example.com"
```

### Column Values (Single Row)

```yaml
- name: "Check aggregate values"
  kind: postgres
  query: "SELECT COUNT(*) as total, SUM(amount) as sum FROM orders"
  expect:
    column_values:
      total: 42
      sum: 1500
```

### Contains/Not Contains

```yaml
# Verify specific rows exist
- name: "Admin users exist"
  kind: postgres
  query: "SELECT email, role FROM users WHERE role = 'admin'"
  expect:
    contains:
      - email: "admin@example.com"
        role: "admin"

# Verify specific rows don't exist
- name: "Deleted user is gone"
  kind: postgres
  query: "SELECT * FROM users"
  expect:
    not_contains:
      - email: "deleted@example.com"
```

### Complex Test Flow

```yaml
tests:
  # HTTP request to create user
  - name: "Create user via API"
    kind: http
    target: api
    request:
      method: POST
      path: /users
      body: '{"name": "Bob", "email": "bob@example.com"}'
    expect:
      status_code: 201

  # Verify in database
  - name: "User exists in DB"
    kind: postgres
    query: "SELECT name, email FROM users WHERE email = 'bob@example.com'"
    expect:
      row_count: 1
      rows:
        - name: "Bob"
          email: "bob@example.com"

  # Delete via API
  - name: "Delete user"
    kind: http
    target: api
    request:
      method: DELETE
      path: /users/bob@example.com

  # Verify deleted
  - name: "User removed from DB"
    kind: postgres
    query: "SELECT * FROM users WHERE email = 'bob@example.com'"
    expect:
      no_rows: true
```

## Features

### Type Compatibility

The plugin automatically handles type conversions:

- Numeric types (int, int64, float64) are compared by value
- Strings are compared exactly
- Booleans are compared exactly
- `[]byte` values are converted to strings

### Fixture Interpolation

You can use fixtures in your queries:

```yaml
fixtures:
  - user_id: 123

tests:
  - name: "Query specific user"
    kind: postgres
    query: "SELECT * FROM users WHERE id = {{ user_id }}"
    expect:
      row_count: 1
```

### Verbose Mode

Run with `-v` flag to see:
- SQL queries being executed
- Result row counts
- Column names
- Returned data

```bash
ene run postgres_test.yaml -v
```

## Testing

Run the plugin tests:

```bash
go test ./plugins/postgrestest/... -v
```

## Dependencies

- `github.com/lib/pq` - PostgreSQL driver
- `github.com/exapsy/ene/e2eframe` - Core testing framework
- `gopkg.in/yaml.v3` - YAML parsing

## License

Same as the parent ene project.