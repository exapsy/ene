# ENE Documentation

Welcome to the ENE (End-to-End) testing framework documentation.

## Overview

ENE is a Docker-based end-to-end testing framework that allows you to spin up complete test environments with databases, services, and mocked APIs to validate your applications through comprehensive integration tests.

## Documentation Structure

### 📖 [Quick Reference](QUICK_REFERENCE.md)
A cheat sheet with common commands, configurations, and patterns. Perfect for quick lookups.

**Use this when you:**
- Need a quick reminder of syntax
- Want to copy-paste common patterns
- Need to reference available options quickly

### 📘 [CLI Usage Guide](CLI_USAGE.md)
Complete guide to using the ENE command-line interface.

**Covers:**
- Installation and prerequisites
- All CLI commands and options
- Running tests with various configurations
- Generating reports (HTML, JSON)
- CI/CD integration
- Shell completion setup
- Troubleshooting common issues

**Use this when you:**
- Are setting up ENE for the first time
- Need to understand CLI flags and options
- Want to integrate ENE into CI/CD pipelines
- Need help debugging test failures

### 📗 [Configuration Reference](CONFIGURATION_REFERENCE.md)
Detailed reference for test suite configuration files (`suite.yml`).

**Covers:**
- Complete YAML schema documentation
- All unit types (HTTP, MongoDB, PostgreSQL, MinIO, etc.)
- Test types and assertions
- Fixtures and variable interpolation
- Size and duration formats
- Complete examples with explanations

**Use this when you:**
- Are writing or modifying test configurations
- Need to understand specific configuration options
- Want to know what assertions are available
- Need to reference service variables

## Quick Start

### 1. Install ENE

```bash
# Clone and build
cd ene
go build -o ene main.go

# Optional: Add to PATH
sudo mv ene /usr/local/bin/
```

### 2. Create Your First Test

```bash
# Scaffold a new test suite
ene scaffold-test my-first-test --tmpl=httpmock

# This creates: tests/my-first-test/suite.yml
```

### 3. Validate Configuration

```bash
# Check if configuration is valid
ene dry-run --verbose
```

### 4. Run Tests

```bash
# Run all tests
ene

# Run specific test
ene --suite=my-first-test --verbose
```

## Common Use Cases

### Testing a REST API
```bash
# Create test with HTTP service
ene scaffold-test api-test --tmpl=http

# Edit tests/api-test/suite.yml to define your tests
# Run the tests
ene --suite=api-test --verbose
```

### Testing with Database
```bash
# Create test with MongoDB
ene scaffold-test db-test --tmpl=mongo,http

# Add migrations in tests/db-test/db.js
# Run the tests
ene --suite=db-test --verbose
```

### Mocking External Services
```bash
# Create test with HTTP mock
ene scaffold-test mock-test --tmpl=httpmock

# Define mock responses in suite.yml
# Run the tests
ene --suite=mock-test
```

### Integration Testing
```bash
# Create comprehensive test
ene scaffold-test integration --tmpl=mongo,http,httpmock

# Configure all services and their interactions
# Run in parallel for speed
ene --suite=integration --parallel
```

## Features

### 🐳 Docker-Based
- Isolated test environments
- No local setup required
- Reproducible tests across machines

### 🔌 Multiple Service Types
- HTTP services (your applications)
- HTTP mocks (external API simulation)
- MongoDB databases
- PostgreSQL databases
- MinIO object storage
- Easily extensible with plugins

### ✅ Rich Assertions
- JSONPath-based body assertions
- Header assertions with regex support
- MinIO state verification
- Type checking and comparisons

### 📊 Reporting
- Pretty console output
- HTML reports for sharing
- JSON reports for CI/CD integration

### ⚡ Performance
- Parallel test execution
- Smart container caching
- Configurable timeouts
- Health check support

### 🔧 Developer Friendly
- YAML configuration
- Variable interpolation
- Fixture support
- Shell completion
- Dry-run validation

## Examples

Example test configurations are available in the `../examples/` directory:

- `minio_example.yaml` - MinIO object storage testing
- `minio_simple_state_test.yaml` - Simple MinIO state verification
- `mongodb-migrations/` - MongoDB with migrations

## Architecture

```
┌─────────────────────────────────────────────────┐
│                  ENE CLI                        │
│  (Orchestrates test execution)                  │
└─────────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────┐
│               Test Suites                       │
│  (suite.yml configuration files)                │
└─────────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────┐
│            Docker Containers                    │
│  (Services, databases, mocks)                   │
└─────────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────┐
│              Test Execution                     │
│  (HTTP requests, assertions)                    │
└─────────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────┐
│               Reports                           │
│  (Console, HTML, JSON)                          │
└─────────────────────────────────────────────────┘
```

## Prerequisites

- **Go 1.18+** - For building ENE
- **Docker** - For running test containers
- **mongosh** (optional) - For MongoDB JavaScript migrations

## Support & Troubleshooting

### Getting Help

1. **Check the docs**: Start with the [Quick Reference](QUICK_REFERENCE.md)
2. **Validate config**: Run `ene dry-run --verbose`
3. **Enable debug**: Use `--debug --verbose` flags
4. **Check Docker**: Ensure Docker is running with `docker ps`

### Common Issues

| Issue | Solution |
|-------|----------|
| Cannot connect to Docker | Ensure Docker daemon is running |
| Port already in use | Change `app_port` in configuration |
| Container timeout | Increase `startup_timeout` value |
| Invalid configuration | Run `ene dry-run --verbose` to see errors |
| Tests fail inconsistently | Try without `--parallel` flag |

### Debug Commands

```bash
# Validate all configurations
ene dry-run --verbose

# Run with full debug output
ene --debug --verbose --suite=failing-test

# Check Docker containers
docker ps -a

# View container logs
docker logs <container-id>
```

## Project Structure

```
ene/
├── main.go                 # CLI entry point
├── e2eframe/              # Core framework
│   ├── e2eframe.go        # Main test runner
│   ├── test_schema.json   # Configuration schema
│   └── ...
├── plugins/               # Service type plugins
│   ├── httpunit/
│   ├── httpmockunit/
│   ├── mongounit/
│   ├── postgresunit/
│   └── miniounit/
├── tests/                 # Your test suites
│   └── <suite-name>/
│       └── suite.yml
├── docs/                  # This documentation
│   ├── README.md
│   ├── CLI_USAGE.md
│   ├── CONFIGURATION_REFERENCE.md
│   └── QUICK_REFERENCE.md
└── examples/              # Example configurations
```

## Contributing

Contributions are welcome! When contributing:

1. Ensure all tests pass: `ene --verbose`
2. Validate configurations: `ene dry-run --verbose`
3. Update documentation for new features
4. Add examples for complex features

## License

[Insert License Information]

## Additional Resources

- **Main README**: `../README.md` - Project overview
- **Examples**: `../examples/` - Sample configurations
- **Source Code**: `../e2eframe/` - Framework implementation

---

**Last Updated**: 2024
**Documentation Version**: 1.0.0
**ENE Version**: dev