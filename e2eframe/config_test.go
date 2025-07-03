package e2eframe_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
	"microservice-var/cmd/e2e/e2eframe"
)

// dummyUnit implements ConfigUnit for testing.
func TestUnmarshalYAML_RequiresAllFields(t *testing.T) {
	var cfg e2eframe.TestSuiteConfigV1

	// missing kind, units, tests, target
	data := `
name: mysuite
`
	err := yaml.Unmarshal([]byte(data), &cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "kind is required")
}
