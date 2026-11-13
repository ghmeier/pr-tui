package agentplans

// Doc is one apl document.
type Doc struct {
	// File is relative to the checkout's .apl directory.
	File  string
	Type  string
	Title string
	// Status is empty when the document has none.
	Status  string
	Tags    []string
	Created string
	// Updated is the date as apl prints it, YYYY-MM-DD.
	Updated string
	// Branches are the branches the document was linked to with apl link.
	Branches []string
	// PullRequestNumbers are the pull requests the document's text mentions, each once, largest
	// first.
	PullRequestNumbers []int
}

// Row is a row of the board.
type Row struct {
	// Repository is the base name of the checkout.
	Repository string
	// Path is the absolute path of the document's file.
	Path string
	Doc  Doc
}

// Key is the repository and file, which stay the same across refreshes.
func (row Row) Key() string { return row.Repository + "/" + row.Doc.File }
