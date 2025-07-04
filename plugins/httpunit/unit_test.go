package httpplugin_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	httpplugin "github.com/exapsy/ene/plugins/httpunit"
	"github.com/stretchr/testify/assert"
)

func TestNewHTTPUnit_Errors(t *testing.T) {
	// missing command
	_, err := httpplugin.New(map[string]any{
		"dockerfile": "Dockerfile",
		"port":       8080,
		"name":       "svc",
	})
	assert.Error(t, err)

	// missing dockerfile
	_, err = httpplugin.New(map[string]any{
		"command": []any{"echo"},
		"port":    8080,
		"name":    "svc",
	})
	assert.Error(t, err)
}

func TestHTTPUnit_FullFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// prepare a simple Python HTTP server Dockerfile
	tmp := t.TempDir()
	df := filepath.Join(tmp, "Dockerfile")

	err := os.WriteFile(df, []byte(`
FROM python:3.11-slim
EXPOSE 8080
CMD ["python3","-m","http.server","8080"]
`), 0o644)
	if err != nil {
		t.Fatalf("write Dockerfile: %v", err)
	}

	cfg := map[string]any{
		"name":            "pyhttp",
		"command":         []any{"python3", "-m", "http.server", "8080"},
		"dockerfile":      df,
		"port":            8080,
		"healthcheck":     "/",
		"env_file":        "",
		"startup_timeout": "30s",
	}

	u, err := httpplugin.New(cfg)
	assert.NoError(t, err)
	assert.Equal(t, "pyhttp", u.Name())

	// start container and wait for the server to respond
	assert.NoError(t, u.Start(ctx, nil))
	assert.NoError(t, u.WaitForReady(ctx))

	// endpoint should include localhost
	ep := u.LocalEndpoint()
	assert.Contains(t, ep, "http://localhost:")

	// Get works for host and port
	h, err := u.Get("host")
	assert.NoError(t, err)
	assert.Equal(t, "localhost", h)

	p, err := u.Get("port")
	assert.NoError(t, err)
	assert.Equal(t, "8080", p)

	// stop the container
	assert.NoError(t, u.Stop())
}
