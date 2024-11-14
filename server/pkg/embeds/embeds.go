package embeds

func Codeblock(code string) string {
	return "```bash\n" + code + "\n```"
}

type Field struct {
	Name  string
	Value string
}

func DisplayFieldList(fields []Field) string {
	var result string
	for _, field := range fields {
		result += field.Name + "\n" + field.Value + "\n"
	}
	return result
}

func Heading(title string) string {
	return "### " + title
}
