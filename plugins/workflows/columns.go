package workflows

import (
	"time"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/ui"
)

func textColumn(header string, maxWidth int, flexible bool, text func(row plugin.Row) string) plugin.Column {
	return plugin.Column{Header: header, MaxWidth: maxWidth, Flexible: flexible, Cell: func(row plugin.Row, _ any) []ui.Segment {
		return ui.PlainSegments(text(row))
	}}
}

func runDuration(run Run) string {
	return durationText(run.duration(time.Now()))
}
