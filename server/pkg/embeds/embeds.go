package embeds

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

func Heading(title string) string {
	return "### " + title
}
