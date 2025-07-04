package e2eframe_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/exapsy/ene/e2eframe"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

type fakeUnit struct{}

func (f *fakeUnit) Name() string { return "u1" }
func (f *fakeUnit) Start(ctx context.Context, opts *e2eframe.UnitStartOptions) error {
	return nil
}
func (f *fakeUnit) WaitForReady(ctx context.Context) error                   { return nil }
func (f *fakeUnit) Stop() error                                              { return nil }
func (f *fakeUnit) ExternalEndpoint() string                                 { return "" }
func (f *fakeUnit) LocalEndpoint() string                                    { return "" }
func (f *fakeUnit) Get(key string) (string, error)                           { return "", nil }
func (f *fakeUnit) GetEnvRaw(_ *e2eframe.GetEnvRawOptions) map[string]string { return nil }
func (f *fakeUnit) SetEnvs(env map[string]string)                            {}

type fakeTest struct {
	NameField string `yaml:"name"`
}

func (f *fakeTest) Name() string                           { return f.NameField }
func (f *fakeTest) Kind() string                           { return "fake" }
func (f *fakeTest) Initialize(ts e2eframe.TestSuite) error { return nil }

func (f *fakeTest) Run(
	ctx context.Context,
	opts *e2eframe.TestSuiteTestRunOptions,
) (*e2eframe.TestResult, error) {
	return &e2eframe.TestResult{TestName: f.Name(), Passed: true}, nil
}

// Go.
func (f *fakeTest) UnmarshalYAML(node *yaml.Node) error {
	// simple walker: expect mapping name -> value
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i].Value
		val := node.Content[i+1]

		switch key {
		case "name":
			if err := val.Decode(&f.NameField); err != nil {
				return err
			}
		case "kind":
			// ignore the kind field
		default:
			return &e2eframe.ConfigKeyNotFoundError{Key: key}
		}
	}

	return nil
}

func setupFakeSuite(t *testing.T) func() {
	t.Helper()
	tempDir := t.TempDir()
	testsDir := filepath.Join(tempDir, e2eframe.TestsDir)
	assert.NoError(t, os.MkdirAll(testsDir, 0o755))

	suiteDir := filepath.Join(testsDir, "s1")
	assert.NoError(t, os.Mkdir(suiteDir, 0o755))

	content := `
kind: e2e_test:v1
name: s1
units:
  - name: u1
    kind: fakeunit
tests:
  - name: t1
    kind: fake
target: u1
`
	assert.NoError(
		t,
		os.WriteFile(filepath.Join(suiteDir, e2eframe.SuiteYamlFile), []byte(content), 0o644),
	)

	oldWd, err := os.Getwd()
	assert.NoError(t, err)
	assert.NoError(t, os.Chdir(tempDir))

	return func() {
		assert.NoError(t, os.Chdir(oldWd))
	}
}
