package table

import (
	"os"

	"github.com/jedib0t/go-pretty/v6/table"
)

// NewTable returns a table writer with the app's standard look: light box
// borders and the given title, writing to standard output.
//
// Callers add headers and rows, then call Render, which prints the table
// directly. Nothing is printed by NewTable itself.
func NewTable(title string) table.Writer {
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleLight)
	t.SetTitle("%s", title)
	return t
}
