package shell

import (
	"github.com/atotto/clipboard"
	"github.com/muesli/termenv"
)

// WriteClipboard puts text on the clipboard. Tests replace it, as they replace Run.
//
// The system clipboard needs a helper such as pbcopy or xclip, which is missing on many
// servers. When it fails, the terminal escape sequence OSC 52 asks the terminal to set its own
// clipboard, which also works over SSH.
var WriteClipboard = func(text string) error {
	if err := clipboard.WriteAll(text); err != nil {
		termenv.DefaultOutput().Copy(text)
	}
	return nil
}
