package e2eframe

import (
	"fmt"
	"regexp"
	"strings"
)

// PrettyError is an interface for errors that can format themselves with colors
// This allows errors to control their own presentation with highlighted diffs
type PrettyError interface {
	error
	// PrettyString returns a formatted error message with ANSI color codes
	// that highlight the difference between expected and actual values
	PrettyString(useColor bool) string
}

// Box drawing characters for error formatting
const (
	boxTopLeft     = "╭"
	boxTopRight    = "╮"
	boxBottomLeft  = "╰"
	boxBottomRight = "╯"
	boxHorizontal  = "─"
	boxVertical    = "│"
	boxTee         = "├"
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
			"  message: \"ok\"",
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

// humanizeFieldPath converts JSON schema field paths to human-readable format.
//
// This function transforms technical JSON path notation (e.g., "units.0") into
// user-friendly messages that include contextual information like names when available.
//
// Examples:
//   - "(root)" -> "root level"
//   - "units.0" with name "mongodb" -> "unit 'mongodb' (units[0])"
//   - "tests.2" without name -> "test at tests[2]"
//   - "fixtures.0" with name "my_fixture" -> "fixture 'my_fixture' (fixtures[0])"
//
// The function attempts to extract the "name" field from array elements in the
// yamlData to provide more context. If no name is found, it falls back to a
// generic format showing just the location.
//
// Parameters:
//   - fieldPath: The JSON schema field path (e.g., "units.0", "tests.1.request")
//   - yamlData: The parsed YAML data structure to extract names and context from
//
// Returns:
//
//	A human-readable string describing the location of the field
func humanizeFieldPath(fieldPath string, yamlData interface{}) string {
	// Handle root level
	if fieldPath == "(root)" {
		return "root level"
	}

	// Parse the path to extract array indices and field names
	parts := strings.Split(fieldPath, ".")

	// Try to get context from the YAML data
	if len(parts) >= 2 {
		arrayName := parts[0]
		if len(parts[1]) > 0 && parts[1][0] >= '0' && parts[1][0] <= '9' {
			// This is an array index
			index := parts[1]

			// Try to get the name from the YAML data
			if dataMap, ok := yamlData.(map[string]interface{}); ok {
				if array, ok := dataMap[arrayName].([]interface{}); ok {
					if idx := parseIndex(index); idx >= 0 && idx < len(array) {
						if item, ok := array[idx].(map[string]interface{}); ok {
							// Check for old format with "name" field
							if name, ok := item["name"].(string); ok {
								singularName := strings.TrimSuffix(arrayName, "s")
								return fmt.Sprintf("%s '%s' (%s[%s])", singularName, name, arrayName, index)
							}
							// Check for new fixture format (single-key map)
							if arrayName == "fixtures" && len(item) == 1 {
								for key := range item {
									return fmt.Sprintf("fixture '%s' (fixtures[%s])", key, index)
								}
							}
						}
					}
				}
			}

			// Fallback to generic message
			singularName := strings.TrimSuffix(arrayName, "s")
			return fmt.Sprintf("%s at %s[%s]", singularName, arrayName, index)
		}
	}

	// Default: just format with brackets
	result := fieldPath
	for i := 0; i < len(parts); i++ {
		if i > 0 && len(parts[i]) > 0 && parts[i][0] >= '0' && parts[i][0] <= '9' {
			result = strings.Replace(result, "."+parts[i], "["+parts[i]+"]", 1)
		}
	}
	return result
}

// parseIndex converts a string containing digits to an integer index.
//
// This helper function safely parses numeric strings without using strconv,
// providing a simple way to convert array index strings like "0", "1", "42"
// into their integer equivalents.
//
// Parameters:
//   - s: A string containing only digits (e.g., "0", "42", "123")
//
// Returns:
//
//	The parsed integer value, or -1 if the string contains non-digit characters
func parseIndex(s string) int {
	var result int
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		result = result*10 + int(c-'0')
	}
	return result
}

// humanizeSchemaErrorDescription improves the readability of JSON schema error descriptions.
//
// This function transforms technical JSON schema validation error messages into
// more user-friendly versions by:
//   - Quoting field names for clarity
//   - Replacing verbose phrases with concise alternatives
//   - Adding context-specific information
//
// Examples:
//   - "Additional property migrations is not allowed"
//     -> "Field 'migrations' is not allowed for this unit type"
//   - "Additional property env_file is not allowed"
//     -> "Field 'env_file' is not allowed for this unit type"
//
// Other error types (like type mismatches, required fields, etc.) are passed
// through unchanged, as they are already reasonably clear.
//
// Parameters:
//   - description: The original JSON schema error description
//
// Returns:
//
//	An improved, more readable error description
func humanizeSchemaErrorDescription(description string) string {
	// Quote field names in "Additional property X" messages
	if strings.HasPrefix(description, "Additional property ") && strings.Contains(description, " is not allowed") {
		// Extract the field name between "Additional property " and " is not allowed"
		start := len("Additional property ")
		end := strings.Index(description, " is not allowed")
		if end > start {
			fieldName := description[start:end]
			return fmt.Sprintf("Field '%s' is not allowed for this unit type", fieldName)
		}
	}

	// Handle other common patterns
	description = strings.ReplaceAll(description, "Additional property ", "Field '")
	description = strings.ReplaceAll(description, " is not allowed", "' is not allowed")

	return description
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
			"  message: \"ok\"     # ✓ Valid",
		}

	case "missing_path":
		message = "body assertion 'path' field is required"
		suggestions = []string{
			"Add a 'path' field to specify which field to test",
			"The path should be a string value",
		}
		examples = []string{
			"body_asserts:",
			"  status: \"success\"",
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

// FormatValidationError creates a nicely formatted validation error message
func FormatValidationError(message, file string, useColor bool) string {
	colorReset := ""
	colorRed := ""
	colorYellow := ""
	colorBold := ""
	colorGray := ""

	if useColor {
		colorReset = "\033[0m"
		colorRed = "\033[31m"
		colorYellow = "\033[33m"
		colorBold = "\033[1m"
		colorGray = "\033[90m"
	}

	var parts []string
	boxWidth := 80

	// Helper to calculate display width (accounting for wide characters)
	displayWidth := func(s string) int {
		// Strip ANSI codes first
		ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)
		stripped := ansiRegex.ReplaceAllString(s, "")

		width := 0
		for _, r := range stripped {
			// Handle wide characters (emoji, some symbols)
			// Based on Unicode East Asian Width property
			if r >= 0x1F300 && r <= 0x1F9FF { // Emoji ranges
				width += 2
			} else if r >= 0x2E80 && r <= 0x9FFF { // CJK
				width += 2
			} else if r >= 0xAC00 && r <= 0xD7AF { // Hangul
				width += 2
			} else if r >= 0xFF00 && r <= 0xFFEF { // Fullwidth forms
				width += 2
			} else {
				width += 1
			}
		}
		return width
	}

	// Helper to wrap text to fit in box (using display width)
	wrapText := func(text string, maxWidth int) []string {
		if displayWidth(text) <= maxWidth {
			return []string{text}
		}

		// For text with ANSI codes, we need to be careful
		ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)

		// Remove ANSI codes for splitting
		stripped := ansiRegex.ReplaceAllString(text, "")

		var lines []string
		currentLine := ""
		currentWidth := 0

		words := strings.Fields(stripped)
		for _, word := range words {
			wordWidth := displayWidth(word)
			spaceWidth := 1

			if currentWidth == 0 {
				// First word on line
				currentLine = word
				currentWidth = wordWidth
			} else if currentWidth+spaceWidth+wordWidth <= maxWidth {
				// Word fits on current line
				currentLine += " " + word
				currentWidth += spaceWidth + wordWidth
			} else {
				// Word doesn't fit, start new line
				lines = append(lines, currentLine)
				currentLine = word
				currentWidth = wordWidth
			}
		}

		if currentLine != "" {
			lines = append(lines, currentLine)
		}

		return lines
	}

	// Helper to create a box line with content
	// Box format: "│ content...padding... │"
	boxLine := func(content string) string {
		contentWidth := displayWidth(content)
		// Total width = 80
		// Format: "│" + " " + content + padding + " " + "│"
		// So: 1 + 1 + contentWidth + padding + 1 + 1 = 80
		// Therefore: padding = 76 - contentWidth
		padding := 76 - contentWidth
		if padding < 0 {
			padding = 0
		}
		return fmt.Sprintf("%s%s%s %s%s %s%s%s",
			colorRed, boxVertical, colorReset, content, strings.Repeat(" ", padding), colorRed, boxVertical, colorReset)
	}

	maxContentWidth := 76

	// Title
	parts = append(parts, "")
	parts = append(parts, fmt.Sprintf("%s%s%s%s%s%s",
		colorRed, colorBold, boxTopLeft, strings.Repeat(boxHorizontal, boxWidth-2), boxTopRight, colorReset))

	titleText := fmt.Sprintf("%s%s✗ VALIDATION FAILED%s", colorBold, colorRed, colorReset)
	parts = append(parts, boxLine(titleText))

	// Empty line
	parts = append(parts, boxLine(""))

	// Split message into lines and wrap if needed
	lines := strings.Split(message, "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		line = strings.TrimSpace(line)

		// Handle bullet points
		if strings.HasPrefix(line, "•") || strings.HasPrefix(line, "-") {
			// Replace - with •
			if strings.HasPrefix(line, "-") {
				line = "•" + line[1:]
			}

			// Wrap long lines
			wrappedLines := wrapText(line, maxContentWidth)
			for i, wrappedLine := range wrappedLines {
				if i == 0 {
					content := fmt.Sprintf("%s%s%s", colorYellow, wrappedLine, colorReset)
					parts = append(parts, boxLine(content))
				} else {
					// Indent continuation lines
					content := fmt.Sprintf("%s  %s%s", colorYellow, wrappedLine, colorReset)
					parts = append(parts, boxLine(content))
				}
			}
		} else {
			// Wrap long lines
			wrappedLines := wrapText(line, maxContentWidth)
			for _, wrappedLine := range wrappedLines {
				parts = append(parts, boxLine(wrappedLine))
			}
		}
	}

	// File context
	if file != "" {
		parts = append(parts, boxLine(""))
		fileContent := fmt.Sprintf("%s📄 %s%s", colorGray, file, colorReset)
		parts = append(parts, boxLine(fileContent))
	}

	parts = append(parts, fmt.Sprintf("%s%s%s%s%s%s",
		colorRed, colorBold, boxBottomLeft, strings.Repeat(boxHorizontal, boxWidth-2), boxBottomRight, colorReset))
	parts = append(parts, "")

	return strings.Join(parts, "\n")
}
