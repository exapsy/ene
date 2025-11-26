# Debug Feature - Quick Reference

## Enable Debug Output

### Suite Level (All Tests)
```yaml
kind: e2e_test:v1
name: my-suite
debug: true  # ← All tests show debug output

tests:
  - name: "test 1"
  - name: "test 2"
```

### Test Level (Specific Tests)
```yaml
kind: e2e_test:v1
name: my-suite

tests:
  - name: "test 1"
    # No debug
  
  - name: "test 2"
    kind: http
    debug: true  # ← Only this test shows debug
```

### Override Suite Level
```yaml
kind: e2e_test:v1
name: my-suite
debug: true  # Default: debug all

tests:
  - name: "test 1"
    # Shows debug (suite default)
  
  - name: "quiet test"
    debug: false  # ← Override: no debug
```

## Priority Order

1. **CLI `-v`** (overrides everything)
2. **Test-level `debug`**
3. **Suite-level `debug`**
4. **Default** (false)

## CLI Usage

```bash
# Normal: use YAML debug settings
ene run test.yaml

# Force debug for ALL tests (overrides YAML)
ene run test.yaml -v

# Re-run with debug on failure
ene run test.yaml || ene run test.yaml -v
```

## What Gets Logged

### HTTP Tests
```
=== HTTP Request ===
POST http://api:8080/users
Headers:
  Content-Type: application/json
Body:
{"name":"Alice"}
====================

=== HTTP Response ===
Status: 201 Created
Body:
{"id":1,"name":"Alice"}
=====================
```

### Postgres Tests
```
=== Postgres Query ===
SELECT * FROM users WHERE id = 1
======================

=== Query Results ===
Row count: 1
Columns: [id name email]
Row 1: map[id:1 name:Alice email:alice@example.com]
=====================
```

## Common Patterns

### Debug a Failing Test
```yaml
tests:
  - name: "working test"
    kind: http
    # No debug
  
  - name: "failing test"
    kind: http
    debug: true  # ← Debug this one
```

### Debug During Development
```yaml
kind: e2e_test:v1
name: new-feature
debug: true  # See everything while building

tests:
  # All tests show debug
```

Remove `debug: true` before committing.

### Debug in CI (Smart)
```bash
# Run normally, re-run with debug if failed
ene run tests.yaml || ene run tests.yaml -v
```

## Supported Test Types

| Test Kind | Debug Support |
|-----------|---------------|
| `http` | ✅ Yes |
| `postgres` | ✅ Yes |
| `minio` | ❌ Not yet |

## Quick Tips

✅ **DO**: Use test-level debug for troubleshooting
✅ **DO**: Use suite-level debug during development
✅ **DO**: Use `-v` flag in CI for failures
❌ **DON'T**: Commit suite-level `debug: true` (too noisy)

## Examples

Full example: `examples/debug_example.yaml`
Full docs: `docs/DEBUG_FEATURE.md`
