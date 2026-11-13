package pullrequests

import (
	"fmt"
	"time"
)

type Repository struct {
	Owner string
	Name  string
}

func (repository Repository) FullName() string {
	return repository.Owner + "/" + repository.Name
}

// CIState is the combined result of a pull request's checks. A pull request whose state has
// not been loaded yet has no CIState at all, which is why PullRequest.CIState is a pointer.
type CIState int

const (
	CIStateNone CIState = iota
	CIStatePassing
	CIStateFailing
	CIStateRunning
)

type ReviewDecision int

const (
	ReviewDecisionNone ReviewDecision = iota
	ReviewDecisionApproved
	ReviewDecisionChangesRequested
)

type LineCount struct {
	Additions int
	Deletions int
}

type PullRequest struct {
	Repository     Repository
	Number         int
	Title          string
	URL            string
	Author         string
	IsBot          bool
	IsDraft        bool
	HeadCommit     string
	AutoMerge      bool
	InMergeQueue   bool
	LineCount      *LineCount
	ReviewDecision ReviewDecision
	CIState        *CIState
	UpdatedAt      time.Time
	RequestedUsers []string
	RequestedTeams []string
}

func (pr PullRequest) label() string {
	return fmt.Sprintf("%s #%d", pr.Repository.Name, pr.Number)
}

// CommentSide says which version of a file a review comment is about.
type CommentSide int

const (
	CommentSideNew CommentSide = iota
	CommentSideOld
)

// CommentTarget is the diff line a review comment attaches to. The side is old for a line
// that exists only in the old file, and new otherwise. A reply names the comment it answers.
type CommentTarget struct {
	Path        string
	Line        int
	Side        CommentSide
	ReplyTo     int64
	ReplyAuthor string
}

func (target CommentTarget) label() string {
	version := "new"
	if target.Side == CommentSideOld {
		version = "old"
	}
	return fmt.Sprintf("%s line %d (%s)", target.Path, target.Line, version)
}

// ReviewComment is one comment on a line of a diff. A comment whose code has since changed
// has no Line.
type ReviewComment struct {
	ID        int64
	InReplyTo int64
	Path      string
	Line      int
	Side      CommentSide
	Author    string
	Body      string
	CreatedAt time.Time
}
