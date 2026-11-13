package agentplans

import (
	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/shell"
)

// openInEditor opens only the document's file, because a document has no other files that
// belong with it.
func (settings settings) openInEditor(row Row) plugin.Effect {
	command := append(append([]string{}, settings.editor.Command...), row.Path)
	return plugin.Effect{
		Notice: "Opening " + row.Doc.File + " in " + settings.editor.Name + "…",
		Command: func() error {
			_, err := shell.Run(command...)
			return err
		},
	}
}
