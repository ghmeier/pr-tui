package workflows

import (
	"slices"
	"strconv"

	"github.com/ghmeier/pr-tui/internal/plugin"
	"github.com/ghmeier/pr-tui/internal/ui"
)

// QueueRow is a pull request in a merge queue as a row of a merge queue section. Other plugins
// that add columns or actions to the board receive these.
type QueueRow struct {
	Entry QueueEntry
}

// Key is the pull request's URL, which stays the same across refreshes.
func (row QueueRow) Key() string {
	return row.Entry.URL
}

func mergeQueueBoard(provider Provider, configured section) plugin.Board {
	entryOf := func(row plugin.Row) QueueEntry { return row.(QueueRow).Entry }
	return plugin.Board{
		Name:    boardName(configured.title),
		Title:   configured.title,
		Tab:     tabTitle,
		RowNoun: "queued pull requests",
		Load:    func() ([]plugin.Row, error) { return loadMergeQueue(provider, configured) },
		Busy:    func(rows []plugin.Row) bool { return len(rows) > 0 },
		Columns: []plugin.Column{
			textColumn("Checks", 0, false, func(row plugin.Row) string { return entryOf(row).Checks.mark() }),
			textColumn("Repo", 0, false, func(row plugin.Row) string { return entryOf(row).Repository.Name }),
			textColumn("#", 0, false, func(row plugin.Row) string { return strconv.Itoa(entryOf(row).Position) }),
			textColumn("PR", 0, false, func(row plugin.Row) string { return "#" + strconv.Itoa(entryOf(row).Number) }),
			textColumn("Title", 0, true, func(row plugin.Row) string { return entryOf(row).Title }),
			textColumn("State", 0, false, func(row plugin.Row) string { return entryOf(row).State.word() }),
			textColumn("Waiting", 0, false, func(row plugin.Row) string { return waitingText(entryOf(row)) }),
			textColumn("ETA", 0, false, func(row plugin.Row) string { return estimateText(entryOf(row)) }),
			textColumn("Author", 20, false, func(row plugin.Row) string { return entryOf(row).Author }),
		},
		Actions: []plugin.Action{
			{Key: "o", Label: "open", Run: func(selection plugin.Selection) plugin.Effect {
				return openInBrowser(entryOf(selection.Row).URL)
			}},
		},
		Facets: []plugin.Facet{{
			Title: "Repository", OptionsFromRows: true,
			Value: func(row plugin.Row) string { return entryOf(row).Repository.FullName() },
		}},
	}
}

// loadMergeQueue lists the queues of the section's repositories in the order the repositories
// are configured, each from its front.
func loadMergeQueue(provider Provider, configured section) ([]plugin.Row, error) {
	entries, err := inEach(configured.repositories, func(repository Repository) ([]QueueEntry, error) {
		queue, err := provider.MergeQueue(repository)
		if err != nil {
			return nil, err
		}
		slices.SortStableFunc(queue, func(left, right QueueEntry) int { return left.Position - right.Position })
		return queue, nil
	})
	if err != nil {
		return nil, err
	}
	return rowsOf(entries, func(entry QueueEntry) plugin.Row { return QueueRow{Entry: entry} }), nil
}

func waitingText(entry QueueEntry) string {
	if entry.EnqueuedAt.IsZero() {
		return ""
	}
	return ui.AgeText(entry.EnqueuedAt)
}

// estimateText is empty when the forge gave no estimate.
func estimateText(entry QueueEntry) string {
	estimate := durationText(entry.EstimatedTimeToMerge)
	if estimate == "" {
		return ""
	}
	return "~" + estimate
}
