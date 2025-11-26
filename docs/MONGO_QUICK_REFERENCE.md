# MongoDB Test Plugin - Quick Reference

Quick reference for the MongoDB test plugin. See [MONGO_TESTS.md](./MONGO_TESTS.md) for full documentation.

## Basic Structure

```yaml
kind: e2e_test:v1
name: my-mongo-test

units:
  - kind: mongo
    name: testdb
    image: mongo:6
    app_port: 27017

target: testdb

tests:
  - name: "test name"
    kind: mongo
    collection: collection_name
    filter: '{"field": "value"}'  # or pipeline
    expect:
      # expectations here
```

## Query Types

### Find Query (JSON String)
```yaml
- name: "Find active users"
  kind: mongo
  collection: users
  filter: '{"status": "active", "age": {"$gte": 18}}'
  expect:
    min_document_count: 1
```

### Find Query (YAML Structure)
```yaml
- name: "Find active users"
  kind: mongo
  collection: users
  filter:
    status: active
    age:
      $gte: 18
  expect:
    min_document_count: 1
```

### Aggregation Pipeline (JSON String)
```yaml
- name: "Count by status"
  kind: mongo
  collection: orders
  pipeline: '[{"$group": {"_id": "$status", "count": {"$sum": 1}}}]'
  expect:
    min_document_count: 1
```

### Aggregation Pipeline (YAML Array)
```yaml
- name: "Count by status"
  kind: mongo
  collection: orders
  pipeline:
    - $match:
        year: 2024
    - $group:
        _id: "$status"
        count: { $sum: 1 }
  expect:
    min_document_count: 2
```

## Common Expectations

### Document Count
```yaml
expect:
  document_count: 5              # Exact count
  min_document_count: 1          # At least 1
  max_document_count: 100        # At most 100
  no_documents: true             # Zero documents
```

### Collection Existence
```yaml
expect:
  collection_exists: "users"
```

### Exact Documents
```yaml
expect:
  document_count: 1
  documents:
    - _id: 1
      name: "Alice"
      email: "alice@example.com"
```

### Field Values (Single Document)
```yaml
expect:
  field_values:
    total: 42
    amount: 1500
```

### Contains/Not Contains
```yaml
expect:
  contains:
    - email: "admin@example.com"
      role: "admin"
  not_contains:
    - email: "deleted@example.com"
```

## Fixture Interpolation

### In Filters
```yaml
fixtures:
  - user_id: "123"

tests:
  - name: "Query user"
    kind: mongo
    collection: users
    filter: '{"_id": "{{ user_id }}"}'
    expect:
      document_count: 1
```

### In Expectations
```yaml
fixtures:
  - expected_email: "test@example.com"

tests:
  - name: "Verify email"
    kind: mongo
    collection: users
    filter: '{}'
    expect:
      contains:
        - email: "{{ expected_email }}"
```

## Per-Test Target Override

```yaml
units:
  - kind: mongo
    name: main_db
  - kind: mongo
    name: analytics_db

tests:
  - name: "Check main DB"
    kind: mongo
    target: main_db
    collection: users
    filter: '{}'
    expect:
      min_document_count: 1
      
  - name: "Check analytics DB"
    kind: mongo
    target: analytics_db
    collection: events
    filter: '{}'
    expect:
      min_document_count: 10
```

## Common Patterns

### Test API + Database
```yaml
tests:
  - name: "Create user"
    kind: http
    target: api
    request:
      method: POST
      path: /users
      body: '{"name": "Bob"}'
      
  - name: "Verify in DB"
    kind: mongo
    target: testdb
    collection: users
    filter:
      name: "Bob"
    expect:
      document_count: 1
```

### Test Data Integrity
```yaml
- name: "No orphaned orders"
  kind: mongo
  collection: orders
  pipeline:
    - $lookup:
        from: users
        localField: user_id
        foreignField: _id
        as: user
    - $match:
        user: { $size: 0 }
  expect:
    no_documents: true
```

### Test Constraints
```yaml
- name: "No negative amounts"
  kind: mongo
  collection: orders
  filter:
    amount:
      $lt: 0
  expect:
    no_documents: true
```

## Debug Mode

Enable verbose output for a specific test:
```yaml
- name: "Debug this test"
  kind: mongo
  debug: true
  collection: users
  filter: '{}'
  expect:
    min_document_count: 1
```

Or run with `-v` flag:
```bash
ene run test.yaml -v
```

## Error Messages

Common errors and solutions:

| Error | Solution |
|-------|----------|
| `no collection provided` | Add `collection` field or use only `collection_exists` |
| `expected X documents, but got Y` | Check your filter/pipeline and data |
| `field 'X' not found in result` | Verify field name exists in documents |
| `collection 'X' does not exist` | Check collection name or run migrations |
| `Failed to connect to MongoDB` | Verify mongo unit is configured and started |

## Test Fields Reference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Test name |
| `kind` | string | Yes | Must be "mongo" |
| `collection` | string | Conditional | Collection to query |
| `filter` | string/object | Optional | Find filter (JSON or YAML) |
| `pipeline` | string/array | Optional | Aggregation pipeline (JSON or YAML) |
| `expect` | object | Yes | Expectations |
| `target` | string | Optional | Override target unit |
| `debug` | boolean | Optional | Enable debug output |

## Expectation Fields Reference

| Field | Type | Description |
|-------|------|-------------|
| `collection_exists` | string | Collection must exist |
| `document_count` | int | Exact document count |
| `min_document_count` | int | Minimum document count |
| `max_document_count` | int | Maximum document count |
| `no_documents` | bool | Zero documents expected |
| `documents` | array | Exact documents match |
| `field_values` | object | Field values (1 doc only) |
| `contains` | array | Must contain these docs |
| `not_contains` | array | Must not contain these docs |

## See Also

- [Full MongoDB Tests Documentation](./MONGO_TESTS.md)
- [PostgreSQL Tests Quick Reference](./POSTGRES_QUICK_REFERENCE.md)
- [Configuration Reference](./CONFIGURATION_REFERENCE.md)