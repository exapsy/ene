# Minio Test Plugin

The Minio test plugin provides comprehensive testing capabilities for Minio object storage operations within the ene E2E testing framework. This plugin allows you to perform assertions on Minio's state, including buckets, objects, content, and metadata.

## Features

- **Bucket Operations**: List, check existence, and validate bucket properties
- **Object Operations**: Upload, download, list, and verify object metadata
- **Content Assertions**: Validate object content with various matching strategies
- **Count Assertions**: Assert on the number of buckets or objects
- **Existence Checks**: Verify if buckets or objects exist
- **Flexible Actions**: Support for all common Minio operations

## Test Kind

```yaml
kind: minio
```

## Supported Actions

| Action | Description | Required Fields |
|--------|-------------|-----------------|
| `list_buckets` | List all buckets | None |
| `list_objects` | List objects in a bucket | `bucket` |
| `get_object` | Download object content | `bucket`, `object` |
| `stat_object` | Get object metadata | `bucket`, `object` |
| `bucket_exists` | Check if bucket exists | `bucket` |
| `object_exists` | Check if object exists | `bucket`, `object` |
| `put_object` | Upload object content | `bucket`, `object` |

## Configuration

### Basic Test Structure

```yaml
tests:
  - name: "Test name"
    kind: minio
    request:
      action: "action_name"
      bucket: "bucket-name"      # Required for most actions
      object: "object-key"       # Required for object operations
      content: "file content"    # For put_object
      prefix: "folder/"          # For list_objects filtering
      timeout: "30s"             # Optional timeout
    expect:
      # Various assertion types (see below)
```

## Assertion Types

### Bucket Assertions

Validate bucket properties and existence:

```yaml
expect:
  bucket_asserts:
    - name: "my-bucket"
      present: true
      created_date: "2024-01-01T00:00:00Z"  # Optional
```

### Object Assertions

Validate object properties and metadata:

```yaml
expect:
  object_asserts:
    - key: "file.txt"
      present: true
      size: 1024                    # File size in bytes
      content_type: "text/plain"    # MIME type
      etag: "d41d8cd98f..."         # Object ETag
```

### Content Assertions

Validate downloaded object content:

```yaml
expect:
  content_assert:
    equals: "exact content match"
    contains: "partial match"
    not_contains: "should not contain this"
    matches: "regex.*pattern"
    not_matches: "avoid.*this"
    min_length: 10
    max_length: 1000
```

### Count Assertions

Assert on the number of items returned:

```yaml
expect:
  count_assert:
    equals: 5
    greater_than: 3
    less_than: 10
    at_least: 2
    at_most: 8
```

### Existence Assertions

Check if resources exist:

```yaml
expect:
  exists_assert:
    bucket: true     # For bucket_exists action
    object: false    # For object_exists action
```

## Examples

### Test Bucket Operations

```yaml
kind: e2e_test:v1
name: bucket-operations-test

units:
  - kind: minio
    name: storage
    buckets:
      - uploads
      - documents

target: storage

tests:
  - name: "List all buckets"
    kind: minio
    request:
      action: list_buckets
    expect:
      count_assert:
        equals: 2
      bucket_asserts:
        - name: uploads
          present: true
        - name: documents
          present: true

  - name: "Check bucket exists"
    kind: minio
    request:
      action: bucket_exists
      bucket: uploads
    expect:
      exists_assert:
        bucket: true

  - name: "Check non-existent bucket"
    kind: minio
    request:
      action: bucket_exists
      bucket: nonexistent
    expect:
      exists_assert:
        bucket: false
```

### Test Object Operations

```yaml
tests:
  - name: "Upload test file"
    kind: minio
    request:
      action: put_object
      bucket: uploads
      object: test.txt
      content: "Hello, World!"
    expect:
      bucket_asserts:
        - name: uploads
          present: true

  - name: "Verify file exists"
    kind: minio
    request:
      action: object_exists
      bucket: uploads
      object: test.txt
    expect:
      exists_assert:
        object: true

  - name: "Download and verify content"
    kind: minio
    request:
      action: get_object
      bucket: uploads
      object: test.txt
    expect:
      content_assert:
        equals: "Hello, World!"
        min_length: 13
        max_length: 13

  - name: "Get file metadata"
    kind: minio
    request:
      action: stat_object
      bucket: uploads
      object: test.txt
    expect:
      object_asserts:
        - key: test.txt
          present: true
          size: 13
```

### Test Object Listing with Prefix

```yaml
tests:
  - name: "Upload files with prefix"
    kind: minio
    request:
      action: put_object
      bucket: uploads
      object: images/photo1.jpg
      content: "fake image content"
    expect:
      bucket_asserts:
        - name: uploads
          present: true

  - name: "List objects with prefix"
    kind: minio
    request:
      action: list_objects
      bucket: uploads
      prefix: images/
    expect:
      count_assert:
        equals: 1
      object_asserts:
        - key: images/photo1.jpg
          present: true
```

### Advanced Content Validation

```yaml
tests:
  - name: "Test content patterns"
    kind: minio
    request:
      action: get_object
      bucket: uploads
      object: config.json
    expect:
      content_assert:
        contains: '"version"'
        not_contains: '"debug": true'
        matches: '"version":\s*"[\d.]+"'
        min_length: 50
        max_length: 1000
```

## Integration with Minio Units

The Minio test plugin automatically connects to Minio units defined in your test suite:

```yaml
units:
  - kind: minio
    name: storage
    access_key: testuser
    secret_key: testpass
    buckets:
      - uploads
      - documents

target: storage  # Tests will use this Minio instance

tests:
  - name: "Test uses storage unit automatically"
    kind: minio
    request:
      action: list_buckets
    expect:
      count_assert:
        at_least: 2
```

## Error Handling

The plugin provides detailed error messages for common issues:

- **Connection failures**: Network or authentication problems
- **Missing resources**: Buckets or objects that don't exist
- **Assertion failures**: Detailed comparison of expected vs actual values
- **Timeout errors**: Operations that exceed specified timeouts

## Best Practices

1. **Use meaningful test names**: Describe what you're testing
2. **Test both positive and negative cases**: Verify expected behavior and error conditions
3. **Use appropriate timeouts**: Set realistic timeouts for operations
4. **Organize tests logically**: Group related operations together
5. **Validate assumptions**: Check bucket existence before object operations
6. **Use prefixes for organization**: Group objects with common prefixes

## Limitations

- Content assertions are limited to text-based files (binary files may not work correctly)
- Large file downloads are limited by memory constraints
- Complex JSON path assertions are not supported (use content patterns instead)
- Date comparisons in bucket assertions require exact RFC3339 format

## Development

The Minio test plugin is located in `ene/plugins/miniotest/` and includes:

- `test.go`: Main plugin implementation
- `test_test.go`: Comprehensive test suite
- Full integration with the ene framework's test interface

To run the plugin tests:

```bash
cd ene
go test ./plugins/miniotest/... -v
```
