package pullrequests

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/ui"
)

type commentThread struct {
	root    ReviewComment
	replies []ReviewComment
}

type threadKey struct {
	path string
	side CommentSide
	line int
}

// groupThreads keys each thread by the diff line it sits on. The provider reports no line for a
// comment whose code has since changed, so those outdated threads are left out.
func groupThreads(comments []ReviewComment) map[threadKey][]*commentThread {
	threads := map[threadKey][]*commentThread{}
	byRoot := map[int64]*commentThread{}
	for _, comment := range comments {
		if comment.InReplyTo == 0 && comment.Line > 0 {
			thread := &commentThread{root: comment}
			byRoot[comment.ID] = thread
			key := threadKey{comment.Path, comment.Side, comment.Line}
			threads[key] = append(threads[key], thread)
		}
	}
	for _, comment := range comments {
		if thread := byRoot[comment.InReplyTo]; thread != nil {
			thread.replies = append(thread.replies, comment)
		}
	}
	return threads
}

var commentSanitizer = strings.NewReplacer("\r", "", "\x1b", "", "\t", "    ")

func (thread commentThread) render(width int) []string {
	var lines []string
	for index, comment := range append([]ReviewComment{thread.root}, thread.replies...) {
		corner := "┌ "
		if index > 0 {
			corner = "├ "
		}
		heading := ui.BoldStyle.Render(comment.Author) + ui.DimStyle.Render(" · "+ui.AgeText(comment.CreatedAt))
		lines = append(lines, ui.YellowStyle.Render(corner)+heading)
		body := commentSanitizer.Replace(strings.TrimSpace(comment.Body))
		for _, text := range strings.Split(ansi.Wrap(body, max(width-2, 10), ""), "\n") {
			lines = append(lines, ui.YellowStyle.Render("│ ")+text)
		}
	}
	return lines
}

func (thread commentThread) replyTarget() CommentTarget {
	root := thread.root
	return CommentTarget{
		Path: root.Path, Line: root.Line, Side: root.Side, ReplyTo: root.ID, ReplyAuthor: root.Author,
	}
}

func (line diffLine) threadKeys(path string) []threadKey {
	switch line.kind {
	case removedLine:
		return []threadKey{{path, CommentSideOld, line.oldNumber}}
	case addedLine:
		return []threadKey{{path, CommentSideNew, line.newNumber}}
	case contextLine:
		return []threadKey{{path, CommentSideNew, line.newNumber}, {path, CommentSideOld, line.oldNumber}}
	}
	return nil
}

// viewRow is one row of the diff pane in the current layout: a diff line, a side-by-side
// pair, or one line of a comment thread anchored to the diff line above it.
type viewRow struct {
	left    int
	right   int
	thread  *commentThread
	comment string
}
