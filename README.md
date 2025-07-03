
<img src="./e2eframe/assets/logo.svg" alt="Logo" width="100">

# Description

This directory contains the end-to-end (e2e) tests for the application. The tests spin up a MongoDB container, run database migrations (if provided), start the server, and execute HTTP requests against it.

![Demo video](./e2eframe/assets/demo.gif)

## Prerequisites

- Go 1.18+ in `PATH` to compile
- Docker daemon running  
- `mongosh` CLI installed (for JS migrations)  

## Directory Structure

```text
.
├── cmd
│   └── e2e                  # TODO: to make framework & separate project instead
│       ├── main.go          # entry point for the test runner
│       └── <...>            # other Go files
└── tests
    └── <suite>            # one folder per test suite
        ├── test.yaml      # suite configuration
        └── db.js         # optional MongoDB migrations
```

- Each `<suite>` folder must contain a `test.yaml` defining `kind`, `name`, and a list of HTTP tests.
- If `db.js` is present, it will be applied before starting the server.

## test.yaml Format

```yaml
kind: e2e:v1
name: <suite-name>

before_all: 'echo "before all hook"'
after_all: 'echo "after all hook"'
before_each: 'echo "before each hook"'
after_each: 'echo "after each hook"'

fixtures:
  - name: content_type_inline
    value: 'application/json'
  - name: ok_file
    file: ./mockdata/ok.json

units:
  - name: app
    kind: http                 # http | mongo | httpmock
    env_file: .env.test        # optional
    env:                       # optional, key-value pairs
      - key=value
    dockerfile: Dockerfile     # optional, either dockerfile or image is required
    image: <image-name>        # optional, either dockerfile or image is required
    app_port: 8080             # required, for inter-container communication
    startup_timeout: 30s       # optional
  - name: mockapp
    kind: httpmock
    app_port: 8080
    routes:
      - path: /healthcheck
        method: GET
        response:
          status: 200
          body:
            data: ok
    

target: mockapp

tests:
  - name: "<test-case-name>"
    type: http                  # subtest type (must be "http")
    request:
      timeout: <duration>         # optional, default is 5s
      path: "/v1/health"        # test path (defaults to test name if omitted)
      method: GET               # HTTP method (default: GET)
      headers:
        key: "value"
      body:
        key: "value"
    expect:
      status: 200               # HTTP status code (default: 200)
      body_assertions:
        - path: "$" # Root document
          equals: "{{ ok_file }}"
        - path: "data.msg"      # JSON path to assert
          equals: "ok"         # expected value
          not_equals: "not ok" # optional
          matches: "ok"        # optional regex match
          not_matches: "not ok" # optional regex not match
          contains: "ok"      # optional substring match
          not_contains: "not ok" # optional substring not match
          present: true # optional, checks if the key is present
      header_assertions: 
        - name: "Content-Type" # header name to assert
          equals: "{{ content_type_inline }}" # expected value
          not_equals: "text/html" # optional
          matches: "application/json" # optional regex match
          not_matches: "text/html" # optional regex not match
          contains: "json" # optional substring match
          not_contains: "html" # optional substring not match

```

- `kind` must be `e2e:v1`.
- `type` is required for each subtest and must be `http`.
- `name` is required for each subtest.
- `request` and `expect` are required for each subtest.

## Running Tests

### Using the script

```sh
# Run all tests
sh scripts/e2e.sh
```

### Directly with Go

```sh
go run ./tests/. \
  --parallel=true \
  --suite=healthcheck,_suffix,prefix_ \
  --verbose
```

- Tests will report passed/failed counts.
- Exits with non-zero code if any test fails.

## Adding New Suites

1. Run `go run ./cmd/e2e/main.go scaffold-test <test-name>`
2. This will create a new folder under `tests/` with a `test.yaml` and optional `db.js`.
3. Edit `test.yaml` to define your test cases.
4. Run the tests

## Debugging Failures

- Failed tests show status mismatches and diff of expected vs. actual JSON.
- Check server logs by enabling `--verbose`.
- Ensure Docker is running and that no port conflicts exist.

## Ideas & Improvements

- Add support for other test types (e.g., gRPC, websocket, file upload).
- Add variable interpolation (env vars, timestamp, random IDs etc.)
- Integrate with CI/CD pipelines for automated testing.
