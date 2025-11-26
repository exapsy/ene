# PostgreSQL Test Plugin - Quick Reference

## Basic Setup

```yaml
kind: e2e_test:v1
name: my-test

units:
  - kind: postgres
    name: db
    image: postgres:15-alpine
    app_port: 5432
    database: testdb
    user: testuser
    password: testpass
    migrations: migrations  # Optional

target: db

tests:
  - name: "My postgres test"
    kind: postgres
    query: "SELECT * FROM users"
    expect:
      row_count: 5
```

## Quick Examples

### Table Existence
```yaml
- name: "Table exists"
  kind: postgres
  expect:
    table_exists: "users"
```

### Row Counts
```yaml
# Exact count
- name: "Exactly 5 users"
  kind: postgres
  query: "SELECT * FROM users"
  expect:
    row_count: 5

# Min/Max range
- name: "Between 1 and 100 users"
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
- name: "Verify specific user"
  kind: postgres
  query: "SELECT id, name, email FROM users WHERE id = 1"
  expect:
    rows:
      - id: 1
        name: "Alice"
        email: "alice@example.com"
```

### Single Row Column Values
```yaml
- name: "Check count"
  kind: postgres
  query: "SELECT COUNT(*) as total FROM users"
  expect:
    column_values:
      total: 42
```

### Contains (Partial Match)
```yaml
- name: "Admin users exist"
  kind: postgres
  query: "SELECT * FROM users"
  expect:
    contains:
      - email: "admin@example.com"
        role: "admin"
      - email: "superadmin@example.com"
        role: "admin"
```

### Not Contains
```yaml
- name: "Deleted user is gone"
  kind: postgres
  query: "SELECT * FROM users"
  expect:
    not_contains:
      - email: "deleted@example.com"
```

### Combining Assertions
```yaml
- name: "Comprehensive check"
  kind: postgres
  query: "SELECT * FROM users WHERE active = true"
  expect:
    min_row_count: 1
    max_row_count: 1000
    contains:
      - email: "admin@example.com"
    not_contains:
      - status: "banned"
```

## HTTP + Postgres Flow

```yaml
tests:
  # 1. Create via API
  - name: "Create user"
    kind: http
    target: api
    request:
      method: POST
      path: /users
      body: '{"name": "Bob", "email": "bob@example.com"}'
    expect:
      status_code: 201

  # 2. Verify in database
  - name: "User in DB"
    kind: postgres
    query: "SELECT * FROM users WHERE email = 'bob@example.com'"
    expect:
      row_count: 1
      rows:
        - name: "Bob"
          email: "bob@example.com"

  # 3. Delete via API
  - name: "Delete user"
    kind: http
    target: api
    request:
      method: DELETE
      path: /users/bob@example.com

  # 4. Verify deleted
  - name: "User deleted"
    kind: postgres
    query: "SELECT * FROM users WHERE email = 'bob@example.com'"
    expect:
      no_rows: true
```

## Common Patterns

### Data Integrity Checks
```yaml
# No orphaned foreign keys
- name: "No orphaned orders"
  kind: postgres
  query: |
    SELECT o.* FROM orders o
    LEFT JOIN users u ON o.user_id = u.id
    WHERE u.id IS NULL
  expect:
    no_rows: true

# Unique constraint validation
- name: "No duplicate emails"
  kind: postgres
  query: |
    SELECT email, COUNT(*) FROM users
    GROUP BY email
    HAVING COUNT(*) > 1
  expect:
    no_rows: true
```

### Aggregate Validations
```yaml
- name: "Order statistics"
  kind: postgres
  query: |
    SELECT 
      COUNT(*) as total,
      SUM(amount) as revenue,
      AVG(amount) as avg_order
    FROM orders
    WHERE status = 'completed'
  expect:
    column_values:
      total: 150
      revenue: 45000
```

### Using Fixtures
```yaml
fixtures:
  - user_id: 123

tests:
  - name: "Query user by ID"
    kind: postgres
    query: "SELECT * FROM users WHERE id = {{ user_id }}"
    expect:
      row_count: 1
```

## Expectation Fields Reference

| Field | Type | Description |
|-------|------|-------------|
| `table_exists` | string | Table name to check |
| `row_count` | int | Exact row count |
| `min_row_count` | int | Minimum rows |
| `max_row_count` | int | Maximum rows |
| `no_rows` | bool | Assert zero rows |
| `rows` | array | Exact row data |
| `column_values` | object | Column values (1 row only) |
| `contains` | array | Must contain these rows |
| `not_contains` | array | Must NOT contain these rows |

## CLI Commands

```bash
# Run tests
ene run postgres_test.yaml

# Verbose output (shows queries and results)
ene run postgres_test.yaml -v

# Run specific test
ene run postgres_test.yaml --filter "test_name"
```

## Tips

✅ **DO:**
- Use descriptive test names
- Write focused, specific queries
- Test data integrity
- Combine with HTTP tests
- Use `-v` for debugging

❌ **AVOID:**
- Overly broad queries (`SELECT *` without WHERE)
- Testing too many things in one query
- Assuming column types (use verbose mode to check)

## Type Compatibility

Numeric types are automatically converted:
- `int`, `int64`, `float64` are compared by value
- `42` matches `int64(42)` matches `float64(42.0)`

## Error Messages

```
# Column value mismatch
row 1, column 'email': expected "alice@example.com", got "alice@test.com"

# Row count mismatch
expected 5 rows, but got 3 rows

# Table not found
table 'users' does not exist

# Row not found
expected row not found in results: map[email:admin@example.com]
```

## More Info

- Full docs: `docs/POSTGRES_TESTS.md`
- Example: `examples/postgres_example.yaml`
- Plugin README: `plugins/postgrestest/README.md`
