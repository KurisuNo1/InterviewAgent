package cli

import (
	"fmt"
	"strings"
)

// FormatTable formats a 2D string slice as a table.
func FormatTable(headers []string, rows [][]string) string {
	if len(headers) == 0 {
		return ""
	}

	// Calculate column widths
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	var sb strings.Builder
	// Headers
	for i, h := range headers {
		sb.WriteString(fmt.Sprintf("%-*s", widths[i]+2, h))
	}
	sb.WriteString("\n")

	// Separator
	for _, w := range widths {
		sb.WriteString(strings.Repeat("-", w+2))
	}
	sb.WriteString("\n")

	// Rows
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				sb.WriteString(fmt.Sprintf("%-*s", widths[i]+2, cell))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
