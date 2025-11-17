package e2eframe

import (
	"testing"
)

func TestHumanizeFieldPath(t *testing.T) {
	tests := []struct {
		name      string
		fieldPath string
		yamlData  interface{}
		want      string
	}{
		{
			name:      "root level",
			fieldPath: "(root)",
			yamlData:  nil,
			want:      "root level",
		},
		{
			name:      "unit with name",
			fieldPath: "units.0",
			yamlData: map[string]interface{}{
				"units": []interface{}{
					map[string]interface{}{
						"name": "mongodb",
						"kind": "mongo",
					},
				},
			},
			want: "unit 'mongodb' (units[0])",
		},
		{
			name:      "unit without name",
			fieldPath: "units.0",
			yamlData: map[string]interface{}{
				"units": []interface{}{
					map[string]interface{}{
						"kind": "mongo",
					},
				},
			},
			want: "unit at units[0]",
		},
		{
			name:      "test with name",
			fieldPath: "tests.2",
			yamlData: map[string]interface{}{
				"tests": []interface{}{
					map[string]interface{}{"name": "test1"},
					map[string]interface{}{"name": "test2"},
					map[string]interface{}{"name": "ping"},
				},
			},
			want: "test 'ping' (tests[2])",
		},
		{
			name:      "fixture with name",
			fieldPath: "fixtures.0",
			yamlData: map[string]interface{}{
				"fixtures": []interface{}{
					map[string]interface{}{
						"name":  "my_fixture",
						"value": "test",
					},
				},
			},
			want: "fixture 'my_fixture' (fixtures[0])",
		},
		{
			name:      "simple field",
			fieldPath: "name",
			yamlData:  nil,
			want:      "name",
		},
		{
			name:      "nested field",
			fieldPath: "request.body",
			yamlData:  nil,
			want:      "request.body",
		},
		{
			name:      "array index without context",
			fieldPath: "units.5",
			yamlData:  map[string]interface{}{},
			want:      "unit at units[5]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := humanizeFieldPath(tt.fieldPath, tt.yamlData)
			if got != tt.want {
				t.Errorf("humanizeFieldPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseIndex(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{
			name:  "zero",
			input: "0",
			want:  0,
		},
		{
			name:  "single digit",
			input: "5",
			want:  5,
		},
		{
			name:  "multiple digits",
			input: "42",
			want:  42,
		},
		{
			name:  "large number",
			input: "999",
			want:  999,
		},
		{
			name:  "non-numeric",
			input: "abc",
			want:  -1,
		},
		{
			name:  "mixed",
			input: "1a",
			want:  -1,
		},
		{
			name:  "empty",
			input: "",
			want:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseIndex(tt.input)
			if got != tt.want {
				t.Errorf("parseIndex(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestHumanizeSchemaErrorDescription(t *testing.T) {
	tests := []struct {
		name        string
		description string
		want        string
	}{
		{
			name:        "additional property not allowed",
			description: "Additional property migrations is not allowed",
			want:        "Field 'migrations' is not allowed for this unit type",
		},
		{
			name:        "additional property with special chars",
			description: "Additional property env_file is not allowed",
			want:        "Field 'env_file' is not allowed for this unit type",
		},
		{
			name:        "other error types unchanged",
			description: "Invalid type. Expected: string, given: integer",
			want:        "Invalid type. Expected: string, given: integer",
		},
		{
			name:        "required field missing",
			description: "name is required",
			want:        "name is required",
		},
		{
			name:        "multiple words in field name",
			description: "Additional property startup_timeout is not allowed",
			want:        "Field 'startup_timeout' is not allowed for this unit type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := humanizeSchemaErrorDescription(tt.description)
			if got != tt.want {
				t.Errorf("humanizeSchemaErrorDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetailedErrorFormat(t *testing.T) {
	err := &DetailedError{
		Message: "Invalid configuration",
		File:    "/path/to/suite.yml",
		Line:    42,
		Suggestions: []string{
			"Check field names for typos",
			"Verify required fields are present",
		},
		Examples: []string{
			"name: my-test",
			"kind: http",
		},
	}

	formatted := err.Format()

	// Check that all components are present
	if !contains(formatted, "ERROR: Invalid configuration") {
		t.Error("Format() should include error message")
	}
	if !contains(formatted, "File: /path/to/suite.yml (line 42)") {
		t.Error("Format() should include file and line")
	}
	if !contains(formatted, "Suggestions:") {
		t.Error("Format() should include suggestions header")
	}
	if !contains(formatted, "Check field names for typos") {
		t.Error("Format() should include all suggestions")
	}
	if !contains(formatted, "Examples:") {
		t.Error("Format() should include examples header")
	}
	if !contains(formatted, "name: my-test") {
		t.Error("Format() should include all examples")
	}
}

func TestNewValidationError(t *testing.T) {
	err := NewValidationError("Test validation error", "/path/to/file.yml", 10)

	if err.Message != "Test validation error" {
		t.Errorf("Message = %q, want %q", err.Message, "Test validation error")
	}
	if err.File != "/path/to/file.yml" {
		t.Errorf("File = %q, want %q", err.File, "/path/to/file.yml")
	}
	if err.Line != 10 {
		t.Errorf("Line = %d, want %d", err.Line, 10)
	}
	if len(err.Suggestions) == 0 {
		t.Error("Expected suggestions to be populated")
	}
}

func TestBodyAssertErrorUserFriendlyMessage(t *testing.T) {
	err := NewBodyAssertValidationError("empty_path", "body.field", "value", "test.yml", 5)

	msg := err.UserFriendlyMessage()
	if msg == "" {
		t.Error("UserFriendlyMessage() should return non-empty string")
	}

	if !contains(msg, "path") {
		t.Error("UserFriendlyMessage() should mention path for empty_path error")
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
