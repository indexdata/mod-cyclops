package cyclops

import "strings"
import "fmt"
import "github.com/indexdata/ccms"

// Used only for logging responses from CCMS

func respToString(resp *ccms.Response) string {
	var b strings.Builder

	for result := range resp.Results() {
		if result.Status() == "error" {
			b.WriteString(result.Status())
			b.WriteString(": ")
			b.WriteString(result.Message())
		} else {
			renderFields(&b, result.Fields())
			first := true
			for row := range result.Data() {
				if !first {
					b.WriteString("\n")
				}
				first = false
				renderRow(&b, row)
			}
		}
	}

	return b.String()
}

// PRIVATE
func renderFields(b *strings.Builder, fields []ccms.FieldDescription) {
	for i, field := range fields {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(field.Name() + "(" + field.DataType() + ")")
	}

	b.WriteString("\n")
}

// PRIVATE
func renderRow(b *strings.Builder, row ccms.DataRow) {
	for i, value := range row.Values() {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString("\"" + fmt.Sprint(value) + "\"")
	}
}
