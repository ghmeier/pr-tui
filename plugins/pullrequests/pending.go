package pullrequests

import (
	"slices"
	"sync"
)

// pendingReviews holds, for each pull request, the line comments a reviewer has collected for
// one submission, and the review body last written. It is read and changed on the update loop
// and by the command that submits a review, so the mutex is what makes that safe. It lives only
// as long as the program.
type pendingReviews struct {
	mutex   sync.Mutex
	entries map[string]*pendingReview
	// version changes on every modification, so that a screen can tell its drawing is stale.
	version int
}

type pendingReview struct {
	drafts []DraftComment
	body   string
	// submissions counts the reviews submitted successfully, so that a screen can tell that the
	// comments it shows are older than the review.
	submissions int
	// submitting is true while a review of the pull request is being sent.
	submitting bool
}

func newPendingReviews() *pendingReviews {
	return &pendingReviews{entries: map[string]*pendingReview{}}
}

// entry is the pull request's record, made on first use. The caller holds the mutex.
func (reviews *pendingReviews) entry(url string) *pendingReview {
	entry := reviews.entries[url]
	if entry == nil {
		entry = &pendingReview{}
		reviews.entries[url] = entry
	}
	return entry
}

func (draft DraftComment) target() CommentTarget {
	return CommentTarget{Path: draft.Path, Line: draft.Line, Side: draft.Side}
}

// drafts is a copy of the pull request's pending comments, in the order they were added.
func (reviews *pendingReviews) drafts(url string) []DraftComment {
	reviews.mutex.Lock()
	defer reviews.mutex.Unlock()
	return slices.Clone(reviews.entry(url).drafts)
}

// count is how many comments are pending, for a label that is drawn on every update.
func (reviews *pendingReviews) count(url string) int {
	reviews.mutex.Lock()
	defer reviews.mutex.Unlock()
	return len(reviews.entry(url).drafts)
}

func (reviews *pendingReviews) draftFor(url string, target CommentTarget) (DraftComment, bool) {
	reviews.mutex.Lock()
	defer reviews.mutex.Unlock()
	for _, draft := range reviews.entry(url).drafts {
		if draft.target() == target {
			return draft, true
		}
	}
	return DraftComment{}, false
}

// save adds the draft, or replaces the one for the same line and side, and returns how many
// comments are pending.
func (reviews *pendingReviews) save(url string, draft DraftComment) int {
	reviews.mutex.Lock()
	defer reviews.mutex.Unlock()
	entry := reviews.entry(url)
	if index := slices.IndexFunc(entry.drafts, func(existing DraftComment) bool { return existing.target() == draft.target() }); index >= 0 {
		entry.drafts[index] = draft
	} else {
		entry.drafts = append(entry.drafts, draft)
	}
	reviews.version++
	return len(entry.drafts)
}

func (reviews *pendingReviews) remove(url string, target CommentTarget) {
	reviews.mutex.Lock()
	defer reviews.mutex.Unlock()
	entry := reviews.entry(url)
	entry.drafts = slices.DeleteFunc(entry.drafts, func(draft DraftComment) bool { return draft.target() == target })
	reviews.version++
}

// body is the review body last written, which a declined or failed submission leaves in place.
func (reviews *pendingReviews) body(url string) string {
	reviews.mutex.Lock()
	defer reviews.mutex.Unlock()
	return reviews.entry(url).body
}

func (reviews *pendingReviews) setBody(url, body string) {
	reviews.mutex.Lock()
	defer reviews.mutex.Unlock()
	reviews.entry(url).body = body
	reviews.version++
}

// beginSubmission marks the pull request's review as being sent, and says whether it was free
// to be. Checking and marking happen together so that two submissions cannot both start.
func (reviews *pendingReviews) beginSubmission(url string) bool {
	reviews.mutex.Lock()
	defer reviews.mutex.Unlock()
	entry := reviews.entry(url)
	if entry.submitting {
		return false
	}
	entry.submitting = true
	return true
}

func (reviews *pendingReviews) endSubmission(url string) {
	reviews.mutex.Lock()
	defer reviews.mutex.Unlock()
	reviews.entry(url).submitting = false
}

// submitted removes the drafts that were sent and clears the body they were sent with. A draft
// added or edited, or a body written, while the review was being submitted is not what was
// sent, so it stays.
func (reviews *pendingReviews) submitted(url string, sent Review) {
	reviews.mutex.Lock()
	defer reviews.mutex.Unlock()
	entry := reviews.entry(url)
	entry.drafts = slices.DeleteFunc(entry.drafts, func(draft DraftComment) bool { return slices.Contains(sent.Comments, draft) })
	if entry.body == sent.Body {
		entry.body = ""
	}
	entry.submissions++
	reviews.version++
}

func (reviews *pendingReviews) submissions(url string) int {
	reviews.mutex.Lock()
	defer reviews.mutex.Unlock()
	return reviews.entry(url).submissions
}

func (reviews *pendingReviews) currentVersion() int {
	reviews.mutex.Lock()
	defer reviews.mutex.Unlock()
	return reviews.version
}
