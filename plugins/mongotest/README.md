# MongoDB Test Plugin

This plugin provides MongoDB database testing capabilities for the ene e2e testing framework.

## Overview

The `mongotest` plugin allows you to:

- Execute MongoDB queries (find operations and aggregation pipelines)
- Assert document counts (exact, min/max ranges, or zero)
- Verify exact document data
- Check collection existence
- Assert specific field values
- Test if results contain or don't contain specific documents
- Validate data integrity and relationships
- Support both JSON string and YAML structure queries

## Installation

This plugin is automatically registered when you import it in your main application:

```go
import _ "github.com/exapsy/ene/plugins/mongotest"
```

## Usage

### Basic Example with Find Query

```yaml
kind: e2e_test:v1
name: my-test

units:
  - kind: mongo
    name: testdb
    image: mongo:6
    app_port: 27017

target: testdb

tests:
  - name: "Check active users"
    kind: mongo
    collection: users
    filter: '{"status": "active"}'
    expect:
      document_count: 5
```

### Example with Aggregation Pipeline

```yaml
tests:
  - name: "Count users by role"
    kind: mongo
    collection: users
    pipeline:
      - $group:
          _id: "$role"
          count: { $sum: 1 }
    expect:
      min_document_count: 2
```

## Configuration

### Test Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique test identifier |
| `kind` | string | Yes | Must be `"mongo"` |
| `collection` | string | Conditional | MongoDB collection name (required unless only checking `collection_exists`) |
| `filter` | string/object | Optional | MongoDB filter for find operations (JSON string or YAML structure) |
| `pipeline` | string/array | Optional | MongoDB aggregation pipeline (JSON string or YAML array) |
| `expect` | object | Yes | Expectations to verify |
| `target` | string | Optional | Override suite-level target unit |
| `debug` | boolean | Optional | Enable debug output for this test |

### Expectation Options

| Field | Type | Description |
|-------|------|-------------|
| `collection_exists` | string | Verify a collection exists |
| `document_count` | integer | Assert exact document count |
| `min_document_count` | integer | Assert minimum document count |
| `max_document_count` | integer | Assert maximum document count |
| `no_documents` | boolean | Assert query returns zero documents |
| `documents` | array[object] | Assert exact document data matches |
| `field_values` | object | Assert field values (single document only) |
| `contains` | array[object] | Assert results contain these documents |
| `not_contains` | array[object] | Assert results don't contain these documents |

## Query Formats

The plugin supports both **JSON strings** (easy copy-paste from MongoDB tools) and **YAML structures** (natural YAML syntax).

### Filter (Find) Operations

#### JSON String Format
```yaml
- name: "Find active users"
  kind: mongo
  collection: users
  filter: '{"status": "active", "age": {"$gte": 18}}'
  expect:
    min_document_count: 1
```

#### YAML Structure Format
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

### Pipeline (Aggregation) Operations

#### JSON String Format
```yaml
- name: "Aggregate by status"
  kind: mongo
  collection: orders
  pipeline: '[{"$group": {"_id": "$status", "count": {"$sum": 1}}}]'
  expect:
    min_document_count: 1
```

#### YAML Array Format
```yaml
- name: "Aggregate by status"
  kind: mongo
  collection: orders
  pipeline:
    - $match:
        year: 2024
    - $group:
        _id: "$status"
        count: { $sum: 1 }
    - $sort:
        count: -1
  expect:
    min_document_count: 1
```

## Examples

### Collection Existence Check

```yaml
- name: "Verify users collection exists"
  kind: mongo
  expect:
    collection_exists: "users"
```

### Document Count Assertions

```yaml
# Exact count
- name: "Verify 5 users"
  kind: mongo
  collection: users
  filter: '{}'
  expect:
    document_count: 5

# Range
- name: "Verify user count in range"
  kind: mongo
  collection: users
  filter: '{}'
  expect:
    min_document_count: 1
    max_document_count: 100

# No documents
- name: "No orphaned records"
  kind: mongo
  collection: orders
  filter:
    user_id:
      $nin: [1, 2, 3]  # assuming valid user IDs
  expect:
    no_documents: true
```

### Exact Document Data

```yaml
- name: "Verify user details"
  kind: mongo
  collection: users
  filter:
    _id: 1
  expect:
    document_count: 1
    documents:
      - _id: 1
        name: "Alice"
        email: "alice@example.com"
```

### Field Values (Single Document)

```yaml
- name: "Check aggregate values"
  kind: mongo
  collection: orders
  pipeline:
    - $group:
        _id: null
        total: { $sum: 1 }
        total_amount: { $sum: "$amount" }
  expect:
    field_values:
      total: 42
      total_amount: 1500
```

### Contains/Not Contains

```yaml
# Verify specific documents exist
- name: "Admin users exist"
  kind: mongo
  collection: users
  filter:
    role: admin
  expect:
    contains:
      - email: "admin@example.com"
        role: "admin"

# Verify specific documents don't exist
- name: "Deleted user is gone"
  kind: mongo
  collection: users
  filter: '{}'
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
    kind: mongo
    target: testdb
    collection: users
    filter:
      email: "bob@example.com"
    expect:
      document_count: 1
      documents:
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
    kind: mongo
    target: testdb
    collection: users
    filter:
      email: "bob@example.com"
    expect:
      no_documents: true
```

## Advanced Features

### Per-Test Target Override

Test multiple MongoDB databases in the same suite:

```yaml
units:
  - kind: mongo
    name: main_db
    image: mongo:6
    
  - kind: mongo
    name: analytics_db
    image: mongo:6
    
  - kind: http
    name: api
    # ... config

target: api

tests:
  - name: "Check API endpoint"
    kind: http
    # uses default target: api
    
  - name: "Check main database"
    kind: mongo
    target: main_db
    collection: users
    filter: '{}'
    expect:
      min_document_count: 1
      
  - name: "Check analytics database"
    kind: mongo
    target: analytics_db
    collection: events
    filter: '{}'
    expect:
      min_document_count: 10
```

### Fixture Interpolation

Use fixtures to make tests dynamic and reusable:

#### Fixtures in Filters

```yaml
fixtures:
  - user_id: "123"
  - user_email: "test@example.com"
  - min_age: 18

tests:
  - name: "Query specific user by ID"
    kind: mongo
    collection: users
    filter: '{"_id": "{{ user_id }}"}'
    expect:
      document_count: 1
      
  - name: "Query by email"
    kind: mongo
    collection: users
    filter:
      email: "{{ user_email }}"
    expect:
      document_count: 1
      
  - name: "Query adults"
    kind: mongo
    collection: users
    filter:
      age:
        $gte: "{{ min_age }}"
    expect:
      min_document_count: 1
```

#### Fixtures in Pipelines

```yaml
fixtures:
  - target_role: "admin"
  - min_count: 5

tests:
  - name: "Aggregate by role"
    kind: mongo
    collection: users
    pipeline:
      - $match:
          role: "{{ target_role }}"
      - $group:
          _id: "$department"
          count: { $sum: 1 }
      - $match:
          count:
            $gte: {{ min_count }}
    expect:
      min_document_count: 1
```

#### Fixtures in Expectations

```yaml
fixtures:
  - admin_email: "admin@example.com"
  - admin_role: "admin"
  - expected_count: 10

tests:
  - name: "Verify admin exists"
    kind: mongo
    collection: users
    filter:
      role: admin
    expect:
      documents:
        - email: "{{ admin_email }}"
          role: "{{ admin_role }}"
          
  - name: "Verify count matches"
    kind: mongo
    collection: users
    filter: '{}'
    expect:
      field_values:
        total: "{{ expected_count }}"
        
  - name: "Admin in results"
    kind: mongo
    collection: users
    filter: '{}'
    expect:
      contains:
        - email: "{{ admin_email }}"
          role: "{{ admin_role }}"
```

#### Fixtures in Collection Names

```yaml
fixtures:
  - collection_name: "users_test"

tests:
  - name: "Check dynamic collection"
    kind: mongo
    collection: "{{ collection_name }}"
    filter: '{}'
    expect:
      min_document_count: 1
```

#### Loading Fixtures from Files

```yaml
fixtures:
  - test_data:
      file: "fixtures/user_data.json"

tests:
  - name: "Verify user data"
    kind: mongo
    collection: users
    filter:
      email: "{{ test_data.email }}"
    expect:
      documents:
        - email: "{{ test_data.email }}"
          name: "{{ test_data.name }}"
```

### Complex Fixture Usage

```yaml
fixtures:
  - test_user_id: "user123"
  - test_user_name: "Charlie"
  - test_user_email: "charlie@example.com"

tests:
  # Create via API
  - name: "Create test user"
    kind: http
    target: api
    request:
      method: POST
      path: /users
      body: |
        {
          "id": "{{ test_user_id }}",
          "name": "{{ test_user_name }}",
          "email": "{{ test_user_email }}"
        }
    
  # Verify in MongoDB
  - name: "Verify user in database"
    kind: mongo
    target: testdb
    collection: users
    filter:
      _id: "{{ test_user_id }}"
    expect:
      document_count: 1
      documents:
        - _id: "{{ test_user_id }}"
          name: "{{ test_user_name }}"
          email: "{{ test_user_email }}"
```

## Best Practices

### 1. Use Specific Queries

Avoid overly broad queries in tests:

```yaml
# Good - specific query
- name: "Check admin count"
  kind: mongo
  collection: users
  filter:
    role: admin
  expect:
    document_count: 3

# Less ideal - too broad
- name: "Check all users"
  kind: mongo
  collection: users
  filter: '{}'
  expect:
    min_document_count: 1
```

### 2. Test Data Integrity

Use MongoDB queries to verify relationships and constraints:

```yaml
- name: "No orders without users"
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

### 3. Combine with HTTP Tests

Test the full flow: API → Database:

```yaml
- name: "Create order"
  kind: http
  target: api
  request:
    method: POST
    path: /orders
    body: '{"user_id": "123", "amount": 50}'
    
- name: "Order exists in DB"
  kind: mongo
  collection: orders
  filter:
    user_id: "123"
  expect:
    contains:
      - amount: 50
```

### 4. Use Collection Existence First

Check collection exists before querying:

```yaml
- name: "Users collection exists"
  kind: mongo
  expect:
    collection_exists: "users"
    
- name: "Query users"
  kind: mongo
  collection: users
  filter: '{}'
  expect:
    min_document_count: 1
```

### 5. Test Edge Cases

```yaml
- name: "No negative amounts"
  kind: mongo
  collection: orders
  filter:
    amount:
      $lt: 0
  expect:
    no_documents: true
    
- name: "No future dates"
  kind: mongo
  collection: events
  filter:
    timestamp:
      $gt: "2099-12-31"
  expect:
    no_documents: true
```

### 6. Use Descriptive Test Names

```yaml
# Good names
- name: "Verify admin users have elevated privileges"
- name: "Check orders created today are pending"
- name: "Ensure deleted users have no active sessions"

# Less descriptive
- name: "Check users"
- name: "Test orders"
- name: "Verify data"
```

## Type Compatibility

The plugin automatically handles type conversions:

- Numeric types (int32, int64, float32, float64) are compared by value
- Strings are compared exactly
- Booleans are compared exactly
- BSON types are normalized for comparison

Example:
```yaml
# MongoDB returns int64, but you can compare with int
expect:
  field_values:
    count: 42  # Works even if MongoDB returns int64(42)
```

## Verbose Mode

Run with `-v` flag to see:
- MongoDB queries being executed (filter or pipeline)
- Collection names
- Result document counts
- Returned documents

```bash
ene run mongo_test.yaml -v
```

Example output:
```
=== MongoDB Query ===
Collection: users
Filter: {
  "status": "active"
}
=====================

=== Query Results ===
Document count: 5
Document 1: map[_id:1 name:Alice status:active]
Document 2: map[_id:2 name:Bob status:active]
...
=====================
```

## Error Messages

The plugin provides detailed error messages:

```
Expectation failed: expected 5 documents, but got 3 documents
Expectation failed: field 'email' not found in result
Expectation failed: document 1, field 'status': expected active (string), got pending (string)
Expectation failed: expected document not found in results: map[email:test@example.com]
```

## Troubleshooting

### Query Execution Fails

**Problem:** `find execution failed: ...` or `aggregation execution failed: ...`

**Solutions:**
- Verify your filter/pipeline syntax is valid MongoDB syntax
- Check that field names exist in the collection
- Use verbose mode (`-v`) to see the exact query being executed
- Test your query in MongoDB Compass or mongosh first

### Type Mismatches

**Problem:** `expected 42 (int), got 42 (int64)`

**Solution:** The plugin normalizes numeric types automatically, but if you see this error, the values are actually different. Check your expectations.

### Connection Issues

**Problem:** `Failed to connect to MongoDB: ...`

**Solutions:**
- Verify the mongo unit is properly configured
- Check that `target` points to a valid mongo unit
- Ensure the MongoDB container started successfully
- Check network connectivity between containers

### Collection Not Found

**Problem:** `collection 'xyz' does not exist`

**Solutions:**
- Verify collection name spelling
- Check if migrations ran successfully
- Ensure data was inserted before the test runs
- Use `collection_exists` to verify first

## Related Documentation

- [PostgreSQL Test Plugin](../postgrestest/README.md) - Similar testing for PostgreSQL
- [HTTP Test Plugin](../httptest/README.md) - For testing HTTP APIs
- [MongoDB Unit Plugin](../mongounit/README.md) - For running MongoDB containers

## Dependencies

- `go.mongodb.org/mongo-driver/mongo` - MongoDB Go driver
- `github.com/exapsy/ene/e2eframe` - Core testing framework
- `gopkg.in/yaml.v3` - YAML parsing

## License

Same as the parent ene project.