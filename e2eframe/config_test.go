package e2eframe_test

import (
	"testing"

	"github.com/exapsy/ene/e2eframe"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
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
