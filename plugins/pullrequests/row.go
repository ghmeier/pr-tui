package pullrequests

// Row is a pull request as a row of the board. Other plugins that add columns or actions to
// the board receive these, and read the pull request from them.
type Row struct {
	PullRequest PullRequest
	// Viewer is the login of whoever is running the board, which arrives together with the
	// pull requests. Rows are values that are never changed, because the background fetches
	// of the board's columns read them while the board goes on drawing.
	Viewer string
}

// Key is the pull request's URL, which stays the same across refreshes.
func (row Row) Key() string {
	return row.PullRequest.URL
}
