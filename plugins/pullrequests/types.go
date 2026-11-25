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

// CIState is the combined result of a pull request's checks.
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

// Reviewer is who a review is requested from: a person, or the review team when Team is set. For
// the team, Login is the team's name.
type Reviewer struct {
	Login string
	Team  bool
}

// Description is a pull request's title and the markdown body under it.
type Description struct {
	Title string
	Body  string
}

type PullRequest struct {
	Repository Repository
	Number     int
	Title      string
	URL        string
	Author     string
	IsBot      bool
	IsDraft    bool
	HeadCommit string
	// HeadBranch is the name of the branch the pull request comes from, which other plugins
	// can match their own rows against.
	HeadBranch     string
	AutoMerge      bool
	InMergeQueue   bool
	ReviewDecision ReviewDecision
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

// CommentTarget is the diff line a review comment attaches to, or the whole file at Path. The
// side is old for a line that exists only in the old file, and new otherwise. A reply names the
// comment it answers. For a whole-file target, Line and Side are unused.
type CommentTarget struct {
	Path        string
	Line        int
	Side        CommentSide
	WholeFile   bool
	ReplyTo     int64
	ReplyAuthor string
}

// CommentKind is the kind of value the diff screen offers a written comment as.
const CommentKind = "comment"

// Comment is a comment written on the diff screen, offered to other plugins' actions.
type Comment struct {
	PullRequest PullRequest
	Target      CommentTarget
	Body        string
}

func (target CommentTarget) label() string {
	if target.WholeFile {
		return target.Path + " (whole file)"
	}
	version := "new"
	if target.Side == CommentSideOld {
		version = "old"
	}
	return fmt.Sprintf("%s line %d (%s)", target.Path, target.Line, version)
}

// ReviewEvent is how a submitted review ends.
type ReviewEvent int

const (
	ReviewEventApprove ReviewEvent = iota
	ReviewEventRequestChanges
	ReviewEventComment
)

// ReviewState is the state a submitted review was left in.
type ReviewState int

const (
	// ReviewStateNone marks a conversation entry that is a plain comment, not a review.
	ReviewStateNone ReviewState = iota
	ReviewStateApproved
	ReviewStateChangesRequested
	ReviewStateCommented
)

// ConversationEntry is a comment on the pull request as a whole, or the text of a submitted review.
type ConversationEntry struct {
	Author  string
	IsBot   bool
	Body    string
	Created time.Time
	// Review is the state of a submitted review, or ReviewStateNone for a plain comment.
	Review ReviewState
}

// DraftComment is a line comment waiting in a pending review.
type DraftComment struct {
	Path string
	Line int
	Side CommentSide
	Body string
}

// Review is what a reviewer submits in one go.
type Review struct {
	Event    ReviewEvent
	Body     string
	Comments []DraftComment
}

// ReviewComment is one comment on a line of a diff, or on a whole file. A line comment whose
// code has since changed has no Line. A whole-file comment has no line or side to use.
type ReviewComment struct {
	ID        int64
	InReplyTo int64
	Path      string
	Line      int
	Side      CommentSide
	WholeFile bool
	Author    string
	Body      string
	CreatedAt time.Time
}
