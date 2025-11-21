# PostgreSQL Unit Plugin

The PostgreSQL unit plugin provides a PostgreSQL database container for testing.

## Configuration

```yaml
units:
  - name: postgres
    kind: postgres
    image: postgres:14              # Optional, defaults to postgres:15-alpine
    app_port: 5432                  # Optional, defaults to 5432
    database: testdb                # Optional, defaults to testdb
    user: testuser                  # Optional, defaults to testuser
    password: testpass              # Optional, defaults to testpass
    migrations: ./migrations        # Optional, path to SQL migration files
    startup_timeout: 30s            # Optional, defaults to 30s
```

## Available Variables

The following variables can be used in other units' configuration via interpolation (e.g., `{{ postgres.user }}`):

### Connection Information

| Variable | Description | Example Value |
|----------|-------------|---------------|
| `hostname` | Service name for container-to-container connections | `postgres` |
| `host` | External host (for host machine connections) | `localhost` or `127.0.0.1` |
| `internal_port` / `app_port` | Internal port for container-to-container connections | `5432` |
| `port` | Externally mapped port (for host machine connections) | `54321` |

### Credentials

| Variable | Description | Example Value |
|----------|-------------|---------------|
| `database` | Database name | `testdb` |
| `user` / `username` | Database user | `testuser` |
| `password` | Database password | `testpass` |

### Connection Strings

| Variable | Description | Example Value |
|----------|-------------|---------------|
| `dsn` / `database_url` | External DSN (for host machine) | `postgres://testuser:testpass@localhost:54321/testdb?sslmode=disable` |
| `local_dsn` / `local_database_url` | Internal DSN (for containers) | `postgres://testuser:testpass@postgres:5432/testdb?sslmode=disable` |

## Usage Examples

### Container-to-Container Connection

When connecting from another container (like an HTTP service), use internal connection details:

```yaml
units:
  - name: postgres
    kind: postgres
    database: mydb
    user: myuser
    password: mypass
    
  - name: app
    kind: http
    dockerfile: Dockerfile
    app_port: 8080
    env:
      - DB_HOST={{ postgres.hostname }}
      - DB_PORT={{ postgres.internal_port }}
      - DB_NAME={{ postgres.database }}
      - DB_USER={{ postgres.username }}
      - DB_PASSWORD={{ postgres.password }}
      # Or use the full connection string:
      - DATABASE_URL={{ postgres.local_dsn }}
```

### Migrations

Place your SQL migration files in a directory and reference it:

```yaml
units:
  - name: postgres
    kind: postgres
    migrations: ./postgres_migrations
```

Migration files should be `.sql` files and will be executed in alphabetical order:

```
postgres_migrations/
├── 001_create_users_table.sql
├── 002_create_orders_table.sql
└── 003_add_indexes.sql
```

## Features

- **Automatic health checks**: Waits for PostgreSQL to be ready before proceeding
- **SQL migrations**: Automatically runs SQL files in order
- **Resource limits**: Configured with 512MB memory limit
- **Network isolation**: Runs in a dedicated Docker network with other test units
- **Cleanup**: Automatically stopped and removed after tests complete

## Notes

- The plugin uses the PostgreSQL official Docker images
- Migrations run after the container is healthy but before tests execute
- For container-to-container connections, always use `hostname` and `internal_port`
- For external connections (e.g., debugging from your IDE), use `host` and `port`
