package e2eframe

import (
	"fmt"
	"strings"
)

// DetailedError provides structured error information with helpful context
type DetailedError struct {
	Message     string
	File        string
	Line        int
	Suggestions []string
	Examples    []string
}

func (e *DetailedError) Error() string {
	return e.Message
}

func (e *DetailedError) UserFriendlyMessage() string {
	return e.Message
}

// Format returns a formatted error message with context, suggestions, and examples
func (e *DetailedError) Format() string {
	var parts []string

	// Main error message
	parts = append(parts, fmt.Sprintf("ERROR: %s", e.Message))

	// File and line context
	if e.File != "" {
		if e.Line > 0 {
			parts = append(parts, fmt.Sprintf("File: %s (line %d)", e.File, e.Line))
		} else {
			parts = append(parts, fmt.Sprintf("File: %s", e.File))
		}
	}

	// Suggestions
	if len(e.Suggestions) > 0 {
		parts = append(parts, "")
		parts = append(parts, "Suggestions:")
		for _, suggestion := range e.Suggestions {
			parts = append(parts, fmt.Sprintf("  • %s", suggestion))
		}
	}

	// Examples
	if len(e.Examples) > 0 {
		parts = append(parts, "")
		parts = append(parts, "Examples:")
		for _, example := range e.Examples {
			parts = append(parts, fmt.Sprintf("  %s", example))
		}
	}

	return strings.Join(parts, "\n")
}

// BodyAssertError creates a specialized error for body assertion validation issues
type BodyAssertError struct {
	*DetailedError
	Path      string
	Condition string
	Value     interface{}
	ErrorType string
}

func NewBodyAssertError(message, file string, line int) *DetailedError {
	return &DetailedError{
		Message: message,
		File:    file,
		Line:    line,
		Suggestions: []string{
			"Check that 'path' field is present and not empty",
			"Ensure at least one assertion condition is provided",
			"Verify field types match expected values",
		},
		Examples: []string{
			"body_asserts:",
			"  - path: \"message\"",
			"    equals: \"ok\"",
		},
	}
}

func NewYAMLError(message, file string) *DetailedError {
	return &DetailedError{
		Message: message,
		File:    file,
		Suggestions: []string{
			"Check indentation (use spaces, not tabs)",
			"Ensure quotes around string values with special characters",
			"Verify YAML structure and field names",
			"Check for duplicate keys in the same section",
		},
	}
}

// NewValidationError creates a user-friendly error for general validation issues
func NewValidationError(message, file string, line int) *DetailedError {
	return &DetailedError{
		Message: message,
		File:    file,
		Line:    line,
		Suggestions: []string{
			"Review the field requirements in the documentation",
			"Check for typos in field names",
			"Ensure all required fields are present",
		},
	}
}

// NewBodyAssertValidationError creates a specialized error for body assertion validation
func NewBodyAssertValidationError(errorType, path string, value interface{}, file string, line int) *BodyAssertError {
	var message string
	var suggestions []string
	var examples []string

	switch errorType {
	case "empty_path":
		message = "body assertion path cannot be empty"
		suggestions = []string{
			"Provide a valid JSON path to the field you want to test",
			"Use '$.field_name' for top-level fields",
			"Use 'field.nested_field' for nested objects",
		}
		examples = []string{
			"body_asserts:",
			"  - path: \"message\"     # ✓ Valid",
			"    equals: \"ok\"",
		}

	case "missing_path":
		message = "body assertion 'path' field is required"
		suggestions = []string{
			"Add a 'path' field to specify which field to test",
			"The path should be a string value",
		}
		examples = []string{
			"body_asserts:",
			"  - path: \"status\"",
			"    equals: \"success\"",
		}

	case "no_conditions":
		message = "body assertion must have at least one test condition"
		suggestions = []string{
			"Add an assertion condition like 'equals', 'contains', or 'present'",
			"Check available conditions in the documentation",
		}
		examples = []string{
			"Available conditions:",
			"  - equals: \"expected_value\"",
			"  - contains: \"substring\"",
			"  - present: true",
			"  - size: 5",
			"  - type: \"string\"",
		}

	case "invalid_type":
		message = fmt.Sprintf("invalid 'type' value: %v", value)
		suggestions = []string{
			"Use one of the valid type values",
			"Check spelling and case sensitivity",
		}
		examples = []string{
			"Valid types:",
			"  - type: \"string\"",
			"  - type: \"int\"",
			"  - type: \"float\"",
			"  - type: \"bool\"",
			"  - type: \"array\"",
			"  - type: \"object\"",
		}

	default:
		message = fmt.Sprintf("body assertion validation error: %s", errorType)
		suggestions = []string{
			"Check the body assertion configuration",
			"Verify all required fields are present",
		}
	}

	return &BodyAssertError{
		DetailedError: &DetailedError{
			Message:     message,
			File:        file,
			Line:        line,
			Suggestions: suggestions,
			Examples:    examples,
		},
		Path:      path,
		Condition: errorType,
		Value:     value,
		ErrorType: errorType,
	}
}

// UserFriendlyMessage returns a simple one-line error message for non-debug mode
func (e *BodyAssertError) UserFriendlyMessage() string {
	return e.Message
}
