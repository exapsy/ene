package ui

import (
	"strings"
)

// LogBoxConfig configures the appearance of a log box
type LogBoxConfig struct {
	// Title is the header text above the logs (e.g., "Container logs:")
	Title string
	// Indent is the number of spaces to indent the entire box
	Indent int
	// MaxWidth is the maximum width for text wrapping
	MaxWidth int
	// HighlightErrors enables colored highlighting for ERROR/WARN/etc
	HighlightErrors bool
}

// DefaultLogBoxConfig returns sensible defaults for a log box
func DefaultLogBoxConfig() LogBoxConfig {
	return LogBoxConfig{
		Title:           "Container logs:",
		Indent:          2,
		MaxWidth:        80,
		HighlightErrors: true,
	}
}

// LogBox creates a formatted log output with vertical line borders
// Returns a multi-line string ready to be printed
func LogBox(logs string, config LogBoxConfig) string {
	if logs == "" {
		return ""
	}

	var out strings.Builder
	indent := strings.Repeat(" ", config.Indent)

	// Add title if provided
	if config.Title != "" {
		out.WriteString("\n\n")
		out.WriteString(indent)
		out.WriteString("\033[2m\033[90m") // Dim gray
		out.WriteString(config.Title)
		out.WriteString("\033[0m")
	}

	// Split logs by lines and format each line
	logLines := strings.Split(logs, "\n")

	for _, line := range logLines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Wrap long lines at word boundaries
		maxContentWidth := config.MaxWidth - config.Indent - 2 // Account for indent and "│ "
		if maxContentWidth < 20 {
			maxContentWidth = 20 // Minimum width
		}
		wrappedLines := wrapText(line, maxContentWidth)

		for _, wrappedLine := range wrappedLines {
			// Apply syntax highlighting if enabled
			formattedLine := wrappedLine
			if config.HighlightErrors {
				formattedLine = highlightLogLine(wrappedLine)
			} else {
				formattedLine = "\033[2m\033[90m" + wrappedLine + "\033[0m"
			}

			// Write the line with vertical border
			out.WriteString("\n")
			out.WriteString(indent)
			out.WriteString("\033[2m\033[90m│\033[0m ") // Dim gray vertical line
			out.WriteString(formattedLine)
		}
	}

	return out.String()
}

// highlightLogLine applies color coding based on log level
func highlightLogLine(line string) string {
	upperLine := strings.ToUpper(line)
	lowerLine := strings.ToLower(line)

	// Highlight ERROR/FATAL/panic in red
	if strings.Contains(upperLine, "ERROR") ||
		strings.Contains(upperLine, "FATAL") ||
		strings.Contains(lowerLine, "panic") {
		return "\033[31m" + line + "\033[0m" // Red
	}

	// Highlight WARN in yellow
	if strings.Contains(upperLine, "WARN") {
		return "\033[33m" + line + "\033[0m" // Yellow
	}

	// Default: dim gray for normal logs
	return "\033[2m\033[90m" + line + "\033[0m"
}

// wrapText wraps text to a specified width, breaking at word boundaries
func wrapText(text string, width int) []string {
	if len(text) <= width {
		return []string{text}
	}

	var lines []string
	words := strings.Fields(text)
	if len(words) == 0 {
		// If no words (e.g., single long string), just split it
		for i := 0; i < len(text); i += width {
			end := i + width
			if end > len(text) {
				end = len(text)
			}
			lines = append(lines, text[i:end])
		}
		return lines
	}

	currentLine := words[0]
	for _, word := range words[1:] {
		if len(currentLine)+1+len(word) <= width {
			currentLine += " " + word
		} else {
			lines = append(lines, currentLine)
			currentLine = word
		}
	}
	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return lines
}
