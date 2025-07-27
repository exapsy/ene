# Minio Plugin for ene E2E Testing Framework

The Minio plugin provides comprehensive support for testing object storage scenarios using Minio containers. It follows a **state-based verification approach** rather than action-based testing, making it ideal for verifying storage state after other operations.

## Key Concepts

### State Verification vs Action Testing

Unlike HTTP tests that verify request/response interactions, Minio tests verify the **current state** of storage:

- **HTTP tests**: "Send this request, expect this response"
- **Minio tests**: "Storage should be in this state"

### Typical Workflow

1. **Action phase**: Use HTTP/API tests to perform operations (upload, process, delete files)
2. **Verification phase**: Use Minio tests to verify the storage state matches expectations

## Minio Unit Configuration

```yaml
units:
  - kind: minio
    name: storage
    image: minio/minio:latest          # Optional, defaults to minio/minio:latest
    access_key: testuser               # Optional, defaults to minioadmin
    secret_key: testpass123            # Optional, defaults to minioadmin
    app_port: 9000                     # Optional, defaults to 9000
    console_port: 9001                 # Optional, defaults to 9001
    startup_timeout: 30s               # Optional, defaults to 10s
    buckets:                           # Optional, buckets to create on startup
      - uploads
      - processed
      - archived
    env:                               # Optional, additional environment variables
      MINIO_BROWSER: "on"
      MINIO_DOMAIN: "localhost"
    cmd:                               # Optional, custom command
      - server
      - /data
      - --console-address
      - ":9001"
```

### Available Variables

The Minio unit exposes these variables for use in other units:

- `{{ unit.host }}` - Container host
- `{{ unit.port }}` - Mapped port for Minio API
- `{{ unit.endpoint }}` - Full external endpoint (host:port)
- `{{ unit.local_endpoint }}` - Internal network endpoint
- `{{ unit.access_key }}` - Access key
- `{{ unit.secret_key }}` - Secret key
- `{{ unit.console_port }}` - Mapped console port
- `{{ unit.console_endpoint }}` - Full console endpoint

## Minio State Verification Tests

### Basic Syntax

```yaml
tests:
  - name: "Verify file upload state"
    kind: minio
    verify_state:
      # State verification configuration
```

### Files Exist (Simple)

Verify that specific files exist in storage:

```yaml
verify_state:
  files_exist:
    - "uploads/user123/profile.jpg"
    - "documents/report.pdf"
    - "backups/daily.zip"
```

### Bucket Counts

Verify the number of files in each bucket:

```yaml
verify_state:
  bucket_counts:
    uploads: 2      # exactly 2 files
    processed: 1    # exactly 1 file
    archived: 0     # empty bucket
```

### Required State

Specify what MUST exist with detailed constraints:

```yaml
verify_state:
  required:
    buckets:
      uploads:
        - path: "user123/profile.jpg"
          min_size: "1KB"           # minimum file size
          max_size: "10MB"          # maximum file size
          max_age: "5m"             # uploaded within last 5 minutes
          content_type: "image/jpeg" # MIME type
        - path: "user123/document.pdf"
          min_size: "50KB"
          content_type: "application/pdf"
    
    files:  # Direct file specifications
      - path: "backups/daily.zip"
        min_size: "100MB"
        max_age: "24h"
```

#### Size Formats

Supported size formats:
- `100` or `100B` - bytes
- `1KB` - kilobytes
- `2MB` - megabytes  
- `3GB` - gigabytes
- `1TB` - terabytes

#### Time Formats

Supported duration formats:
- `30s` - seconds
- `5m` - minutes
- `2h` - hours
- `24h` - hours
- `7d` - days (use `168h`)

### Forbidden State

Specify what MUST NOT exist:

```yaml
verify_state:
  forbidden:
    buckets:
      uploads:
        - "temp_*"        # no files matching temp_*
        - "*.tmp"         # no .tmp files
        - "debug_*"       # no debug files
      cache:
        - "*"             # bucket should be completely empty
    
    files:  # Direct file paths that should not exist
      - "uploads/password.txt"
      - "logs/sensitive.log"
```

#### Pattern Matching

Forbidden patterns support glob-style wildcards:
- `*` - matches any characters
- `temp_*` - matches files starting with "temp_"
- `*.tmp` - matches files ending with ".tmp"
- `user*/profile.*` - matches nested patterns

### Constraints

Flexible validation rules for buckets and global storage:

```yaml
verify_state:
  constraints:
    # Bucket-specific constraints
    - bucket: uploads
      file_count: 2                    # exactly 2 files
      max_total_size: "100MB"          # total size limit
      min_total_size: "1MB"            # minimum total size
    
    - bucket: processed
      file_count: ">= 1"               # at least 1 file
      max_total_size: "200MB"
    
    # Global constraints
    - total_buckets: "<= 5"            # at most 5 buckets
    - empty_buckets: "allowed"         # empty buckets are OK
```

#### Constraint Operators

File count and bucket count constraints support:
- `5` - exactly 5
- `>= 5` or `≥ 5` - 5 or more
- `<= 5` or `≤ 5` - 5 or fewer
- `> 5` - more than 5
- `< 5` - fewer than 5

## Complete Example

```yaml
kind: e2e_test:v1
name: file-storage-workflow

units:
  - kind: minio
    name: storage
    buckets: [uploads, processed, archived]
  
  - kind: http
    name: file-api
    env:
      STORAGE_ENDPOINT: "{{ storage.local_endpoint }}"
      STORAGE_ACCESS_KEY: "{{ storage.access_key }}"
      STORAGE_SECRET_KEY: "{{ storage.secret_key }}"

target: file-api

tests:
  # Action: Upload files via API
  - name: "Upload user files"
    kind: http
    request:
      method: POST
      path: /upload
      files:
        profile: "profile.jpg"
        document: "resume.pdf"
    expect:
      status_code: 200

  # Verification: Check storage state
  - name: "Verify files were stored correctly"
    kind: minio
    verify_state:
      # Simple existence check
      files_exist:
        - "uploads/user123/profile.jpg"
        - "uploads/user123/resume.pdf"
      
      # Detailed requirements
      required:
        buckets:
          uploads:
            - path: "user123/profile.jpg"
              min_size: "1KB"
              max_size: "10MB"
              content_type: "image/jpeg"
              max_age: "2m"
            - path: "user123/resume.pdf"
              content_type: "application/pdf"
      
      # No temp files allowed
      forbidden:
        buckets:
          uploads:
            - "temp_*"
            - "*.tmp"
      
      # Count and size constraints
      constraints:
        - bucket: uploads
          file_count: 2
          max_total_size: "50MB"
        - bucket: processed
          file_count: 0  # should be empty
```

## Integration with Other Units

### With HTTP Services

```yaml
- kind: http
  name: api
  env:
    MINIO_ENDPOINT: "{{ storage.local_endpoint }}"
    MINIO_ACCESS_KEY: "{{ storage.access_key }}"
    MINIO_SECRET_KEY: "{{ storage.secret_key }}"
```

### With Database Services

```yaml
tests:
  # Update database
  - name: "Record file upload"
    kind: http
    request:
      method: POST
      path: /files
      body: '{"name": "profile.jpg", "bucket": "uploads"}'
  
  # Verify database state
  - name: "Check database record"
    kind: postgres
    query: "SELECT COUNT(*) FROM files WHERE name = 'profile.jpg'"
    expect:
      rows: [["1"]]
  
  # Verify storage state
  - name: "Check file actually exists in storage"
    kind: minio
    verify_state:
      files_exist:
        - "uploads/profile.jpg"
```

## Best Practices

### 1. Test Storage State, Not Actions

❌ **Don't** test Minio operations directly:
```yaml
# Avoid this approach
- name: "Upload file to Minio"
  kind: minio
  action: put_object  # This was the old approach
```

✅ **Do** verify storage state after operations:
```yaml
# Use this approach
- name: "Upload via API"
  kind: http
  # ... perform upload

- name: "Verify file was stored"
  kind: minio
  verify_state:
    files_exist: ["uploads/file.txt"]
```

### 2. Use Meaningful Test Names

```yaml
# Good: describes what state is being verified
- name: "Verify user profile upload completed successfully"
- name: "Confirm temporary files were cleaned up"
- name: "Check processed files moved to archive"
```

### 3. Combine Simple and Detailed Checks

```yaml
verify_state:
  # Start with simple checks
  files_exist:
    - "uploads/profile.jpg"
  
  # Add detailed constraints as needed
  required:
    buckets:
      uploads:
        - path: "profile.jpg"
          min_size: "1KB"
          content_type: "image/jpeg"
```

### 4. Use Constraints for Business Rules

```yaml
constraints:
  - bucket: uploads
    max_total_size: "1GB"      # Business rule: max 1GB per user
  - bucket: temp
    file_count: 0              # Business rule: temp should be cleaned
  - total_buckets: "<= 10"     # Infrastructure limit
```

### 5. Test Error Conditions

```yaml
# Verify cleanup worked
verify_state:
  forbidden:
    buckets:
      uploads:
        - "*.tmp"          # No temp files
        - "failed_*"       # No failed uploads
        - "corrupt_*"      # No corrupt files
```

## Migration from Old Approach

If you're migrating from the action-based approach:

### Old Approach (Deprecated)
```yaml
- name: "Test file upload"
  kind: minio
  request:
    action: put_object
    bucket: uploads
    object: test.txt
    content: "test"
  expect:
    # complex assertion structure
```

### New Approach
```yaml
# Step 1: Perform action via your API
- name: "Upload file via API"
  kind: http
  request:
    method: POST
    path: /upload
    body: '{"filename": "test.txt", "content": "test"}'

# Step 2: Verify storage state
- name: "Verify file was stored"
  kind: minio
  verify_state:
    files_exist:
      - "uploads/test.txt"
    required:
      buckets:
        uploads:
          - path: "test.txt"
            min_size: "1B"
```

## Troubleshooting

### Common Issues

1. **Files not found**: Check bucket names and file paths
2. **Size mismatches**: Verify size format (KB, MB, GB)
3. **Time constraints**: Ensure max_age is reasonable for test execution time
4. **Pattern matching**: Test forbidden patterns carefully

### Debug Tips

```yaml
# Use simple checks first
verify_state:
  files_exist:
    - "uploads/test.txt"

# Then add constraints incrementally
verify_state:
  files_exist:
    - "uploads/test.txt"
  required:
    buckets:
      uploads:
        - path: "test.txt"
          min_size: "1B"  # Start with minimal constraints
```

### Error Messages

The plugin provides detailed error messages:
- File existence: `"file uploads/test.txt does not exist"`
- Size constraints: `"file size 500 is less than minimum 1024"`
- Count mismatches: `"bucket uploads has 3 files, expected 2"`
- Pattern violations: `"forbidden file pattern *.tmp found: temp_file.tmp"`
