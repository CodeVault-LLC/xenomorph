package embeds

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

const (
	MinusLength = 6
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
	parts := splitIntoChunks(value, maxLength-MinusLength) // Reserve space for reopening/closing codeblocks if needed

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
