# MongoDB Unit Plugin

The MongoDB unit plugin provides MongoDB container support for end-to-end testing with automatic migration execution.

## Features

- 🚀 Automatic MongoDB container startup using testcontainers
- 📦 Configurable MongoDB versions via Docker images
- 🔄 Automatic migration script execution on startup
- ⚡ Smart wait strategy ensuring MongoDB is ready before migrations
- 💾 Memory-optimized configuration for testing environments
- 🔗 Network alias support for inter-container communication

## Configuration

### Basic Configuration

```yaml
units:
  - name: mongodb
    kind: mongo
    image: mongo:6.0
    app_port: 27017
```

### Configuration with Migrations

```yaml
units:
  - name: mongodb
    kind: mongo
    image: mongo:6.0
    migrations: db.js
    app_port: 27017
    startup_timeout: 15s
```

## Configuration Options

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | ✅ Yes | - | Unique identifier for the MongoDB unit |
| `kind` | string | ✅ Yes | - | Must be `mongo` |
| `image` | string | ❌ No | `mongo:6` | Docker image for MongoDB |
| `app_port` | integer | ✅ Yes | - | MongoDB port (typically 27017) |
| `migrations` | string | ❌ No | - | Path to migration JavaScript file (relative to test suite) |
| `startup_timeout` | string | ❌ No | `5s` | Time to wait for container startup |
| `cmd` | array | ❌ No | See below | Custom MongoDB command arguments |
| `env` | map | ❌ No | - | Environment variables |
| `env_file` | string | ❌ No | - | Path to environment file |

### Default Command

If no `cmd` is specified, the following default is used:

```yaml
cmd:
  - mongod
  - --bind_ip_all
  - --wiredTigerCacheSizeGB
  - "0.25"
```

## Migrations

MongoDB migrations use JavaScript files that are executed with `mongosh` (MongoDB Shell) after the container starts and becomes ready.

### Migration File Format

Migration files are standard MongoDB shell scripts written in JavaScript:

```javascript
// Switch to the database
db = db.getSiblingDB('test');

// Create collections
db.createCollection('users');

// Insert data
db.users.insertMany([
  { username: 'testuser1', email: 'test1@example.com', active: true },
  { username: 'testuser2', email: 'test2@example.com', active: true }
]);

// Create indexes
db.users.createIndex({ username: 1 }, { unique: true });
db.users.createIndex({ email: 1 }, { unique: true });

// Print confirmation
print('✅ Migration completed successfully');
```

### Migration Best Practices

1. **Use Explicit Database Selection**: Always switch to your target database using `db.getSiblingDB()`
2. **Idempotent Operations**: Design migrations to be safely re-runnable
3. **Add Logging**: Use `print()` statements to provide feedback
4. **Handle Errors**: Check operation results and provide meaningful messages
5. **Create Indexes**: Define indexes for query performance from the start

### Migration Execution Flow

1. MongoDB container starts
2. Plugin waits for MongoDB to be ready (up to 30 seconds)
3. Migration file is read from the filesystem
4. Migration script is executed via `mongosh --quiet --eval`
5. Exit code is checked; non-zero causes startup to fail
6. Container is ready for tests

### Migration File Location

Migration files are resolved relative to the test suite directory:

```
tests/
  api-tests/
    suite.yml          # Contains: migrations: db.js
    db.js             # ✅ Found here
  shared/
    common-setup.js   # Can reference as: migrations: ../shared/common-setup.js
```

## Usage in Tests

### Accessing MongoDB Connection

```yaml
tests:
  - name: test_database
    kind: http
    request:
      path: /api/users
      method: GET
    expect:
      status_code: 200
```

### Using Variables

The MongoDB unit exposes variables that can be used in your tests:

- `{{ mongodb.host }}` - Container host
- `{{ mongodb.port }}` - Exposed port
- `{{ mongodb.dsn }}` - Full MongoDB connection string

## Examples

### Example 1: Simple Test Database

```yaml
# suite.yml
kind: e2e_test:v1
name: mongo-tests

units:
  - name: mongodb
    kind: mongo
    image: mongo:6.0
    app_port: 27017
    migrations: setup.js

tests:
  - name: test_connection
    kind: http
    request:
      path: /health
      method: GET
```

```javascript
// setup.js
db = db.getSiblingDB('test');
db.createCollection('health_checks');
db.health_checks.insertOne({
  status: 'healthy',
  timestamp: new Date()
});
print('✅ Database initialized');
```

### Example 2: Multi-Collection Setup

```yaml
units:
  - name: mongodb
    kind: mongo
    image: mongo:7.0
    app_port: 27017
    migrations: init-db.js
    startup_timeout: 20s
```

```javascript
// init-db.js
db = db.getSiblingDB('ecommerce');

// Create collections
db.createCollection('products');
db.createCollection('users');
db.createCollection('orders');

// Insert seed data
db.products.insertMany([
  { name: 'Product A', price: 29.99, stock: 100 },
  { name: 'Product B', price: 49.99, stock: 50 }
]);

db.users.insertMany([
  { email: 'user1@test.com', name: 'Test User 1' },
  { email: 'user2@test.com', name: 'Test User 2' }
]);

// Create indexes
db.products.createIndex({ name: 1 });
db.users.createIndex({ email: 1 }, { unique: true });
db.orders.createIndex({ userId: 1, createdAt: -1 });

print('✅ E-commerce database initialized');
print('📦 Products: ' + db.products.countDocuments());
print('👥 Users: ' + db.users.countDocuments());
```

### Example 3: Custom Configuration

```yaml
units:
  - name: mongodb
    kind: mongo
    image: mongo:6.0
    app_port: 27017
    migrations: migrations/seed.js
    startup_timeout: 30s
    cmd:
      - mongod
      - --bind_ip_all
      - --wiredTigerCacheSizeGB
      - "0.5"
      - --quiet
    env:
      MONGO_INITDB_DATABASE: myapp
```

## Troubleshooting

### Migration File Not Found

**Error**: `migration file does not exist: tests/api-tests/db.js`

**Solution**: Ensure the migration file path is relative to the test suite directory and the file exists.

### Migration Script Fails

**Error**: `migration script failed with exit code 1`

**Solution**: 
- Check JavaScript syntax in your migration file
- Verify database operations are valid
- Test the script manually with `mongosh`
- Add `print()` statements for debugging

### MongoDB Not Ready

**Error**: `mongodb did not become ready within 30 seconds`

**Solutions**:
- Increase `startup_timeout` value
- Check Docker resources (memory, CPU)
- Verify MongoDB image is compatible
- Check Docker logs for startup issues

### Connection Refused

**Solution**:
- Ensure `app_port` is set correctly (typically 27017)
- Check network configuration
- Verify MongoDB container started successfully

## Performance Tips

1. **Use Lightweight Images**: Consider `mongo:6-alpine` for faster startup
2. **Optimize Memory**: Default is 512MB, adjust based on your needs
3. **Minimize Migrations**: Keep migration scripts lean for faster execution
4. **Index Later**: Create indexes after bulk inserts for better performance
5. **Reuse Containers**: Configure test framework to reuse containers when possible

## Version Compatibility

| MongoDB Version | Image | Notes |
|----------------|-------|-------|
| 6.0 | `mongo:6.0` | ✅ Recommended, includes mongosh |
| 7.0 | `mongo:7.0` | ✅ Latest stable, includes mongosh |
| 5.0 | `mongo:5.0` | ⚠️ Use `mongo` shell instead of `mongosh` |
| 4.4 | `mongo:4.4` | ⚠️ Use `mongo` shell instead of `mongosh` |

**Note**: MongoDB 5.0+ includes `mongosh` by default. For older versions, you may need to adjust migration execution or use custom images.

## Resource Limits

The plugin automatically configures resource limits for testing:

- **Memory**: 512 MB max
- **Memory Swap**: 512 MB (no additional swap)
- **WiredTiger Cache**: 0.25 GB (default)

These limits ensure MongoDB runs efficiently in CI/CD environments without consuming excessive resources.

## Contributing

To add features or fix bugs in the MongoDB unit plugin:

1. Update `unit.go` with your changes
2. Add tests to verify functionality
3. Update this README with new features
4. Submit a pull request

## Related Plugins

- [PostgreSQL Unit](../postgresunit/README.md) - PostgreSQL database support
- [Minio Unit](../miniounit/README.md) - S3-compatible object storage
- [HTTP Mock Unit](../httpmockunit/README.md) - Mock HTTP services

## License

This plugin is part of the ENE testing framework.