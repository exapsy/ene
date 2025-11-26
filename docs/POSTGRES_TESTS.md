# PostgreSQL Test Plugin

The PostgreSQL test plugin allows you to verify database state and query results in your E2E tests. This is useful for testing data persistence, integrity constraints, and complex business logic that involves database operations.

## Table of Contents

- [Overview](#overview)
- [Basic Usage](#basic-usage)
- [Configuration Options](#configuration-options)
- [Assertion Types](#assertion-types)
- [Fixture Interpolation](#fixture-interpolation)
- [Examples](#examples)
- [Best Practices](#best-practices)

## Overview

The postgres test plugin (`kind: postgres`) enables you to:

- Execute SQL queries against a PostgreSQL database
- Assert row counts (exact, min, max, or none)
- Verify exact row data
- Check if tables exist
- Assert column values for single-row results
- Check if results contain or don't contain specific rows
- Test data integrity and relationships

## Basic Usage

To use the postgres test plugin, you need:

1. A PostgreSQL unit configured in your test suite
2. Set the postgres unit as your target
3. Define tests with `kind: postgres`

```yaml
kind: e2e_test:v1
name: my-postgres-test

units:
  - kind: postgres
    name: testdb
    image: postgres:15-alpine
    app_port: 5432
    database: testdb
    user: testuser
    password: testpass
    migrations: migrations  # Optional: path to SQL migration files

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

## Configuration Options

### Test-Level Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Test name for identification |
| `kind` | string | Yes | Must be `"postgres"` |
| `target` | string | No | Override suite-level target (must be a postgres unit) |
| `query` | string | No* | SQL query to execute |
| `expect` | object | Yes | Expectations to verify |

*Note: `query` is optional only when using `table_exists` assertion alone.

### Expectation Fields

All fields in the `expect` object are optional, but at least one must be specified:

| Field | Type | Description |
|-------|------|-------------|
| `table_exists` | string | Check if a table exists in the database |
| `row_count` | integer | Expect an exact number of rows |
| `min_row_count` | integer | Expect at least this many rows |
| `max_row_count` | integer | Expect at most this many rows |
| `no_rows` | boolean | Assert that the query returns no rows |
| `rows` | array | Expect exact row data (all columns must match) |
| `column_values` | object | Assert column values for single-row results |
| `contains` | array | Assert that results contain these rows (partial match) |
| `not_contains` | array | Assert that results do NOT contain these rows |

## Assertion Types

### 1. Table Existence

Check if a table exists in the database schema:

```yaml
- name: "Verify users table exists"
  kind: postgres
  expect:
    table_exists: "users"
```

### 2. Row Count Assertions

#### Exact Count

```yaml
- name: "Verify exactly 5 users"
  kind: postgres
  query: "SELECT * FROM users"
  expect:
    row_count: 5
```

#### Min/Max Range

```yaml
- name: "Verify user count is within range"
  kind: postgres
  query: "SELECT * FROM users"
  expect:
    min_row_count: 1
    max_row_count: 100
```

#### No Rows

```yaml
- name: "Verify no orphaned records"
  kind: postgres
  query: "SELECT * FROM orders WHERE user_id NOT IN (SELECT id FROM users)"
  expect:
    no_rows: true
```

### 3. Exact Row Data

Verify that the query returns specific rows with exact values:

```yaml
- name: "Verify user data"
  kind: postgres
  query: "SELECT id, name, email FROM users WHERE id = 1"
  expect:
    row_count: 1
    rows:
      - id: 1
        name: "Alice Johnson"
        email: "alice@example.com"
```

**Note:** All specified columns must match exactly. Order of rows matters.

### 4. Column Values (Single Row)

For queries that return a single row, you can assert specific column values:

```yaml
- name: "Check total user count"
  kind: postgres
  query: "SELECT COUNT(*) as total, MAX(created_at) as latest FROM users"
  expect:
    row_count: 1
    column_values:
      total: 42
```

**Note:** This assertion requires exactly one row in the result set.

### 5. Contains Assertion

Check if specific rows exist in the results (partial matching):

```yaml
- name: "Verify admin users exist"
  kind: postgres
  query: "SELECT id, email, role FROM users WHERE role = 'admin'"
  expect:
    contains:
      - email: "admin@example.com"
        role: "admin"
      - email: "superadmin@example.com"
        role: "admin"
```

**Note:** Only the specified columns need to match. Extra columns in results are ignored.

### 6. Not Contains Assertion

Verify that specific rows do NOT exist in the results:

```yaml
- name: "Verify deleted user is gone"
  kind: postgres
  query: "SELECT * FROM users"
  expect:
    not_contains:
      - email: "deleted@example.com"
```

### 7. Combining Assertions

You can combine multiple assertions in a single test:

```yaml
- name: "Comprehensive user check"
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

## Per-Test Target Override

By default, postgres tests use the suite-level `target` to determine which postgres unit to query. However, you can override this on a per-test basis:

```yaml
units:
  - kind: postgres
    name: main_db
    # ... config
  
  - kind: postgres
    name: analytics_db
    # ... config
  
  - kind: http
    name: api
    # ... config

target: api  # Default target for HTTP tests

tests:
  # This test uses the default target (api) - will fail!
  - name: "Query default target"
    kind: postgres
    query: "SELECT * FROM users"
    expect:
      row_count: 1
  
  # This test overrides to use main_db
  - name: "Query main database"
    kind: postgres
    target: main_db  # Override to postgres unit
    query: "SELECT * FROM users"
    expect:
      row_count: 1
  
  # This test uses analytics_db
  - name: "Query analytics database"
    kind: postgres
    target: analytics_db  # Override to different postgres unit
    query: "SELECT COUNT(*) as count FROM events"
    expect:
      min_row_count: 100
```

**Important:** The target (whether suite-level or test-level) must be a postgres unit. If you target an HTTP unit or other non-postgres unit, you'll get an error:

```
failed to get postgres DSN from unit 'api': unknown variable dsn 
(hint: target must be a postgres unit)
```

## Fixture Interpolation

The postgres test plugin fully supports fixture interpolation in both queries and expectations. This allows you to use dynamic values from fixtures in your tests.

### Fixtures in Queries

You can use fixtures directly in your SQL queries:

```yaml
fixtures:
  - user_id: 123
  - user_email: "alice@example.com"
  - min_age: 18

tests:
  - name: "Query specific user"
    kind: postgres
    query: "SELECT * FROM users WHERE id = {{ user_id }}"
    expect:
      row_count: 1
  
  - name: "Query by email"
    kind: postgres
    query: "SELECT * FROM users WHERE email = '{{ user_email }}'"
    expect:
      row_count: 1
  
  - name: "Query users above minimum age"
    kind: postgres
    query: "SELECT * FROM users WHERE age >= {{ min_age }}"
    expect:
      min_row_count: 1
```

### Fixtures in Expectations

Fixtures can also be used in expectation values:

```yaml
fixtures:
  - admin_email: "admin@example.com"
  - admin_role: "admin"
  - expected_count: 5

tests:
  # Use fixtures in exact row data
  - name: "Verify admin user"
    kind: postgres
    query: "SELECT email, role FROM users WHERE role = 'admin'"
    expect:
      rows:
        - email: "{{ admin_email }}"
          role: "{{ admin_role }}"
  
  # Use fixtures in column values
  - name: "Check user count"
    kind: postgres
    query: "SELECT COUNT(*) as total FROM users"
    expect:
      column_values:
        total: "{{ expected_count }}"
  
  # Use fixtures in contains assertions
  - name: "Admin exists"
    kind: postgres
    query: "SELECT * FROM users"
    expect:
      contains:
        - email: "{{ admin_email }}"
          role: "{{ admin_role }}"
  
  # Use fixtures in not_contains assertions
  - name: "Deleted user is gone"
    kind: postgres
    query: "SELECT * FROM users"
    expect:
      not_contains:
        - email: "{{ deleted_user_email }}"
```

### Fixtures in Table Names

You can even use fixtures in table existence checks:

```yaml
fixtures:
  - table_name: "users"

tests:
  - name: "Verify table exists"
    kind: postgres
    expect:
      table_exists: "{{ table_name }}"
```

### Complex Fixture Usage

Combine fixtures in queries and expectations for powerful dynamic testing:

```yaml
fixtures:
  - test_user_id: 42
  - test_user_name: "Bob Smith"
  - test_user_email: "bob@example.com"

tests:
  # Insert via API (fixtures used in HTTP test)
  - name: "Create user"
    kind: http
    target: api
    request:
      method: POST
      path: /users
      body: '{"id": {{ test_user_id }}, "name": "{{ test_user_name }}", "email": "{{ test_user_email }}"}'
  
  # Verify in database using same fixtures
  - name: "User exists in DB"
    kind: postgres
    query: "SELECT id, name, email FROM users WHERE id = {{ test_user_id }}"
    expect:
      row_count: 1
      rows:
        - id: "{{ test_user_id }}"
          name: "{{ test_user_name }}"
          email: "{{ test_user_email }}"
```

### Loading Fixtures from Files

You can also load fixtures from JSON files:

```yaml
fixtures:
  - test_data:
      file: "fixtures/test_users.json"

tests:
  - name: "Verify user data"
    kind: postgres
    query: "SELECT * FROM users WHERE id = {{ test_data.user_id }}"
    expect:
      rows:
        - email: "{{ test_data.email }}"
```

## Examples

### Example 1: Database State After Migrations

```yaml
tests:
  - name: "Verify schema is set up correctly"
    kind: postgres
    expect:
      table_exists: "users"
  
  - name: "Verify tables are empty initially"
    kind: postgres
    query: "SELECT COUNT(*) as count FROM users"
    expect:
      column_values:
        count: 0
```

### Example 2: Testing Data Insertion

```yaml
tests:
  # Insert data via API
  - name: "Create user via API"
    kind: http
    target: api
    request:
      method: POST
      path: /users
      body: '{"name": "Alice", "email": "alice@example.com"}'
    expect:
      status_code: 201
  
  # Verify data in database
  - name: "Verify user was inserted"
    kind: postgres
    query: "SELECT id, name, email FROM users WHERE email = 'alice@example.com'"
    expect:
      row_count: 1
      rows:
        - id: 1
          name: "Alice"
          email: "alice@example.com"
```

### Example 3: Testing Data Integrity

```yaml
tests:
  - name: "Verify no orphaned foreign keys"
    kind: postgres
    query: |
      SELECT o.* FROM orders o
      LEFT JOIN users u ON o.user_id = u.id
      WHERE u.id IS NULL
    expect:
      no_rows: true
  
  - name: "Verify unique constraint"
    kind: postgres
    query: |
      SELECT email, COUNT(*) as count
      FROM users
      GROUP BY email
      HAVING COUNT(*) > 1
    expect:
      no_rows: true
```

### Example 4: Testing Complex Queries

```yaml
tests:
  - name: "Verify user statistics"
    kind: postgres
    query: |
      SELECT 
        COUNT(*) as total_users,
        COUNT(CASE WHEN active = true THEN 1 END) as active_users,
        COUNT(CASE WHEN role = 'admin' THEN 1 END) as admin_users
      FROM users
    expect:
      row_count: 1
      column_values:
        total_users: 150
        active_users: 145
        admin_users: 3
```

### Example 5: Dynamic Testing with Fixtures

Use fixtures for parameterized testing:

```yaml
fixtures:
  - user_id: 42
  - expected_name: "Alice Johnson"
  - expected_email: "alice@example.com"

tests:
  # Create user via API with fixture
  - name: "Create user"
    kind: http
    target: api
    request:
      method: POST
      path: /users
      body: '{"id": {{ user_id }}, "name": "{{ expected_name }}", "email": "{{ expected_email }}"}'
  
  # Query using fixture
  - name: "Query specific user"
    kind: postgres
    query: "SELECT * FROM users WHERE id = {{ user_id }}"
    expect:
      row_count: 1
      rows:
        - id: "{{ user_id }}"
          name: "{{ expected_name }}"
          email: "{{ expected_email }}"
```

## Best Practices

### 1. Use Specific Queries

Write targeted queries that test specific aspects of your data:

```yaml
# Good: Specific and focused
- name: "Verify active admin users"
  kind: postgres
  query: "SELECT COUNT(*) as count FROM users WHERE role = 'admin' AND active = true"
  expect:
    column_values:
      count: 2

# Avoid: Too broad
- name: "Check users"
  kind: postgres
  query: "SELECT * FROM users"
  expect:
    min_row_count: 1
```

### 2. Test Data Integrity

Use postgres tests to verify referential integrity and constraints:

```yaml
- name: "No orphaned comments"
  kind: postgres
  query: |
    SELECT c.* FROM comments c
    WHERE NOT EXISTS (SELECT 1 FROM posts p WHERE p.id = c.post_id)
  expect:
    no_rows: true
```

### 3. Combine with HTTP Tests

Test the full flow: API call → Database state verification:

```yaml
- name: "Delete user"
  kind: http
  target: api
  request:
    method: DELETE
    path: /users/123

- name: "Verify user was soft-deleted"
  kind: postgres
  query: "SELECT deleted_at FROM users WHERE id = 123"
  expect:
    row_count: 1
    # Verify deleted_at is not null (soft delete)
```

### 4. Use Table Existence First

Check table existence before running queries:

```yaml
- name: "Verify migrations ran"
  kind: postgres
  expect:
    table_exists: "users"

- name: "Check user data"
  kind: postgres
  query: "SELECT * FROM users"
  expect:
    min_row_count: 0
```

### 5. Test Edge Cases

Verify boundary conditions and edge cases:

```yaml
- name: "Verify no future dates"
  kind: postgres
  query: "SELECT * FROM events WHERE event_date > CURRENT_DATE + INTERVAL '1 year'"
  expect:
    no_rows: true

- name: "Verify email format"
  kind: postgres
  query: "SELECT * FROM users WHERE email NOT LIKE '%@%'"
  expect:
    no_rows: true
```

### 6. Use Descriptive Test Names

Make your test names clear about what they're verifying:

```yaml
# Good
- name: "Verify exactly 3 admin users exist"
- name: "Check no pending orders older than 30 days"
- name: "Ensure all users have valid email addresses"

# Avoid
- name: "Test 1"
- name: "Check database"
```

## Type Compatibility

The plugin handles type conversions automatically:

- Numeric types (int, int64, float32, float64) are compared by value
- String comparisons are exact
- Boolean comparisons are exact
- `[]byte` values are converted to strings

Example:

```yaml
# These will match correctly even with different numeric types
query: "SELECT COUNT(*) as count FROM users"
expect:
  column_values:
    count: 5  # Will match int64(5) from database
```

## Error Messages

The plugin provides detailed error messages:

```
Expectation failed: row 1, column 'email': expected "alice@example.com" (string), got "alice@test.com" (string)
```

```
Expectation failed: expected 5 rows, but got 3 rows
```

```
Expectation failed: table 'users' does not exist
```

## Troubleshooting

### Query Execution Fails

If your query fails to execute:

1. Check SQL syntax
2. Verify table and column names
3. Check user permissions
4. Run with `-v` flag for verbose output to see the query being executed

### Type Mismatches

If you get type mismatch errors:

1. Check the actual types returned by your query
2. Use type casting in SQL if needed: `SELECT id::text, COUNT(*)::int as count`
3. Remember that COUNT() returns int64

### Connection Issues

If connection fails:

1. Verify the postgres unit is properly configured
2. Check that migrations completed successfully
3. Ensure the target is set to the postgres unit

## Fixture Interpolation Details

### Supported Locations

Fixtures are interpolated in:
- ✅ SQL queries (`query` field)
- ✅ Table names (`table_exists` field)
- ✅ Expected row data (`rows` field)
- ✅ Column values (`column_values` field)
- ✅ Contains assertions (`contains` field)
- ✅ Not contains assertions (`not_contains` field)

### Type Handling

Fixture values are interpolated as strings by default. For numeric comparisons, the plugin's type compatibility handles the conversion automatically:

```yaml
fixtures:
  - user_count: 5  # String "5"

tests:
  - name: "Count matches"
    kind: postgres
    query: "SELECT COUNT(*) as count FROM users"
    expect:
      column_values:
        count: "{{ user_count }}"  # Will match int64(5) from database
```

## Related Documentation

- [PostgreSQL Unit Plugin](./POSTGRES_UNIT.md) - How to configure the postgres unit
- [Configuration Reference](./CONFIGURATION_REFERENCE.md) - Full test suite configuration
- [HTTP Test Plugin](./HTTP_TESTS.md) - For testing APIs that interact with Postgres
- [Fixtures Guide](./FIXTURES.md) - Complete guide to using fixtures