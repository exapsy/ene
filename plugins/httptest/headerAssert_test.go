package httptest_test

import (
	"testing"

	htt "github.com/exapsy/ene/plugins/httptest"
	"github.com/stretchr/testify/require"
)

func TestNewTestHeaderAssert_InvalidConfig(t *testing.T) {
	// Missing name and no assertion fields yields error
	_, err := htt.NewTestHeaderAssert(map[string]any{})
	require.Error(t, err)
}
