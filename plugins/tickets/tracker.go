package tickets

import "strings"

// ticket is one made-up ticket. A real plugin would fetch these from a service.
type ticket struct {
	ID     string
	Title  string
	Status string
}

const (
	statusOpen       = "Open"
	statusInProgress = "In progress"
	statusDone       = "Done"
)

func exampleTickets() []ticket {
	return []ticket{
		{ID: "TCK-101", Title: "Add the sign-in page", Status: statusInProgress},
		{ID: "TCK-102", Title: "Fix the invoice total", Status: statusOpen},
		{ID: "TCK-103", Title: "Retire the old export", Status: statusDone},
	}
}

// tracker is what the plugin's board, column, and actions share. It is never changed after
// setup, because they run on different goroutines.
type tracker struct {
	urlPrefix string
	tickets   []ticket
}

func newTracker(settings settings) *tracker {
	return &tracker{urlPrefix: settings.URLPrefix, tickets: exampleTickets()}
}

func (tracker *tracker) url(target ticket) string {
	return tracker.urlPrefix + target.ID
}

// ticketForBranch finds the first ticket whose id appears in the branch name, ignoring case,
// such as TCK-101 in "feature/tck-101-sign-in".
func (tracker *tracker) ticketForBranch(branch string) (ticket, bool) {
	upperBranch := strings.ToUpper(branch)
	for _, candidate := range tracker.tickets {
		if strings.Contains(upperBranch, candidate.ID) {
			return candidate, true
		}
	}
	return ticket{}, false
}
