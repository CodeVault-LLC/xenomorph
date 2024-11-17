package embeds

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func Codeblock(code string) string {
	return "```python\n" + code + "\n```"
}

type Field struct {
	Name  string
	Value string
}

func DisplayFieldList(fields []Field) string {
	var result string
	for _, field := range fields {
		result += field.Name + ": " + field.Value + "\n"
	}
	return result
}

// Table generates a well-formatted ASCII table with aligned columns.
func Table(headers []string, rows [][]string) string {
	if len(headers) == 0 {
		return ""
	}

	// Find the number of columns
	numColumns := len(headers)

	// Ensure all rows have the same number of columns as the headers
	for i, row := range rows {
		if len(row) < numColumns {
			rows[i] = append(row, make([]string, numColumns-len(row))...)
		} else if len(row) > numColumns {
			rows[i] = row[:numColumns] // Trim extra cells
		}
	}

	// Calculate the maximum width of each column
	columnWidths := make([]int, numColumns)
	for i, header := range headers {
		columnWidths[i] = len(header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > columnWidths[i] {
				columnWidths[i] = len(cell)
			}
		}
	}

	// Helper function to format a single row
	formatRow := func(cells []string) string {
		var formattedCells []string
		for i, cell := range cells {
			formattedCells = append(formattedCells, fmt.Sprintf("%-*s", columnWidths[i], cell))
		}
		return strings.Join(formattedCells, " | ")
	}

	// Build the table
	var result strings.Builder
	result.WriteString(formatRow(headers)) // Add headers
	result.WriteString("\n")
	result.WriteString(strings.Repeat("-", len(formatRow(headers)))) // Add separator
	result.WriteString("\n")
	for _, row := range rows {
		result.WriteString(formatRow(row)) // Add each row
		result.WriteString("\n")
	}

	return result.String()
}

// SplitField splits long values into multiple fields, preserving codeblock formatting if present.
func SplitField(name, value string) []*discordgo.MessageEmbedField {
	const maxLength = 1024
	fields := []*discordgo.MessageEmbedField{}

	if len(value) <= maxLength {
		fields = append(fields, &discordgo.MessageEmbedField{Name: name, Value: value})
		return fields
	}

	// Detect if the value includes a codeblock
	isCodeblock := strings.HasPrefix(value, "```") || strings.HasPrefix(value, "`")

	// If it's a codeblock, remove the wrapping to avoid splitting mid-format
	if isCodeblock {
		value = strings.TrimPrefix(value, "```")
		value = strings.TrimSuffix(value, "```")
		value = strings.TrimPrefix(value, "`")
		value = strings.TrimSuffix(value, "`")
	}

	// Split the content into chunks
	parts := splitIntoChunks(value, maxLength-6) // Reserve space for reopening/closing codeblocks if needed

	// Reapply codeblock formatting to each chunk
	for i, part := range parts {
		fieldName := name
		if i > 0 {
			fieldName = name + " (cont.)"
		}
		if isCodeblock {
			part = "```" + part + "```"
		}
		fields = append(fields, &discordgo.MessageEmbedField{Name: fieldName, Value: part})
	}

	return fields
}

// splitIntoChunks splits a string into smaller chunks of a given size.
func splitIntoChunks(input string, size int) []string {
	var chunks []string
	for len(input) > size {
		chunks = append(chunks, input[:size])
		input = input[size:]
	}
	if len(input) > 0 {
		chunks = append(chunks, input)
	}
	return chunks
}
