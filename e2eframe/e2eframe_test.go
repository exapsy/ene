package e2eframe_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"microservice-var/cmd/e2e/e2eframe"
)

type fakeUnit struct{}

func (f *fakeUnit) Name() string { return "u1" }
func (f *fakeUnit) Start(ctx context.Context, opts *e2eframe.UnitStartOptions) error {
	return nil
}
func (f *fakeUnit) WaitForReady(ctx context.Context) error { return nil }
func (f *fakeUnit) Stop() error                            { return nil }
func (f *fakeUnit) ExternalEndpoint() string               { return "" }
func (f *fakeUnit) LocalEndpoint() string                  { return "" }
func (f *fakeUnit) Get(key string) (string, error)         { return "", nil }
func (f *fakeUnit) GetEnvRaw() map[string]string           { return nil }
func (f *fakeUnit) SetEnvs(env map[string]string)          {}

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
	assert.NoError(t, os.MkdirAll(testsDir, 0755))

	suiteDir := filepath.Join(testsDir, "s1")
	assert.NoError(t, os.Mkdir(suiteDir, 0755))

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
		os.WriteFile(filepath.Join(suiteDir, e2eframe.SuiteYamlFile), []byte(content), 0644),
	)

	oldWd, err := os.Getwd()
	assert.NoError(t, err)
	assert.NoError(t, os.Chdir(tempDir))

	return func() {
		assert.NoError(t, os.Chdir(oldWd))
	}
}

func TestRun_Sequential(t *testing.T) {
	assert := assert.New(t)

	e2eframe.RegisterUnitMarshaller("fakeunit", func(node *yaml.Node) (e2eframe.Unit, error) {
		return &fakeUnit{}, nil
	})
	e2eframe.RegisterTestSuiteTestUnmarshaler(
		"fake",
		func(node *yaml.Node) (e2eframe.TestSuiteTest, error) {
			var ft fakeTest
			if err := node.Decode(&ft); err != nil {
				return nil, err
			}

			return &ft, nil
		},
	)

	cleanup := setupFakeSuite(t)
	defer cleanup()

	ch, err := e2eframe.Run(t.Context(), &e2eframe.RunOpts{Parallel: false})
	require.NoError(t, err)

	var results []e2eframe.TestResult
	for res := range ch {
		results = append(results, res)
	}

	assert.Len(results, 1)
	assert.Equal("t1", results[0].TestName)
	assert.True(results[0].Passed)
}

func TestRun_Parallel(t *testing.T) {
	assert := assert.New(t)

	e2eframe.RegisterUnitMarshaller("fakeunit", func(node *yaml.Node) (e2eframe.Unit, error) {
		return &fakeUnit{}, nil
	})
	e2eframe.RegisterTestSuiteTestUnmarshaler(
		"fake",
		func(node *yaml.Node) (e2eframe.TestSuiteTest, error) {
			var ft fakeTest
			if err := node.Decode(&ft); err != nil {
				return nil, err
			}

			return &ft, nil
		},
	)

	cleanup := setupFakeSuite(t)
	defer cleanup()

	ch, err := e2eframe.Run(context.Background(), &e2eframe.RunOpts{Parallel: true})
	require.NoError(t, err)

	var results []e2eframe.TestResult
	for r := range ch {
		results = append(results, r)
	}

	assert.Len(results, 1)
	assert.Equal("t1", results[0].TestName)
	assert.True(results[0].Passed)
}
