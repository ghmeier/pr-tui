package pullrequests

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/ui"
)

// The names are colour schemes that ship with the syntax highlighter.
var syntaxTheme = func() *chroma.Style {
	if ui.DarkBackground {
		return styles.Get("github-dark")
	}
	return styles.Get("github")
}()

type lineKind int

const (
	metadataLine lineKind = iota
	hunkLine
	contextLine
	addedLine
	removedLine
)

type diffLine struct {
	text      string
	kind      lineKind
	code      []ui.Segment
	oldNumber int
	newNumber int
}

// splitRow is one row of the side-by-side view, as indexes into a file's lines. A side
// with nothing on it is -1, and a line that spans both sides has the same index twice.
type splitRow struct {
	left  int
	right int
}

type diffFile struct {
	path        string
	additions   int
	deletions   int
	lines       []diffLine
	splitRows   []splitRow
	numberWidth int
}

var hunkHeaderPattern = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)`)

var diffLineSanitizer = strings.NewReplacer("\t", "    ", "\r", "", "\x1b", "␛")

// parseDiff splits a unified diff into its files. Lines before a file's first hunk are
// metadata, so a leading + or - only counts as a change once a hunk has started.
func parseDiff(text string) []diffFile {
	var files []diffFile
	inHunk := false
	oldNumber, newNumber := 0, 0
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			path := line
			if index := strings.LastIndex(line, " b/"); index >= 0 {
				path = line[index+3:]
			}
			files = append(files, diffFile{path: path})
			inHunk = false
			continue
		}
		if len(files) == 0 {
			continue
		}
		file := &files[len(files)-1]
		parsed := diffLine{text: diffLineSanitizer.Replace(line)}
		switch {
		case strings.HasPrefix(line, "@@"):
			inHunk = true
			parsed.kind = hunkLine
			if numbers := hunkHeaderPattern.FindStringSubmatch(line); numbers != nil {
				oldNumber, _ = strconv.Atoi(numbers[1])
				newNumber, _ = strconv.Atoi(numbers[2])
			}
		case !inHunk || strings.HasPrefix(line, `\`):
		case strings.HasPrefix(line, "+"):
			file.additions++
			parsed.kind, parsed.newNumber = addedLine, newNumber
			newNumber++
		case strings.HasPrefix(line, "-"):
			file.deletions++
			parsed.kind, parsed.oldNumber = removedLine, oldNumber
			oldNumber++
		default:
			parsed.kind, parsed.oldNumber, parsed.newNumber = contextLine, oldNumber, newNumber
			oldNumber++
			newNumber++
		}
		file.lines = append(file.lines, parsed)
	}
	for index := range files {
		files[index].highlight()
		files[index].pairLines()
	}
	return files
}

// highlight tokenises each hunk as one block of source, so a string or comment that spans
// several lines of the hunk keeps its colour.
func (file *diffFile) highlight() {
	lexer := lexers.Match(file.path)
	if lexer == nil {
		return
	}
	lexer = chroma.Coalesce(lexer)
	start := 0
	for start < len(file.lines) {
		if file.lines[start].kind < contextLine {
			start++
			continue
		}
		end := start
		var source strings.Builder
		for end < len(file.lines) && file.lines[end].kind >= contextLine {
			if text := file.lines[end].text; text != "" {
				source.WriteString(text[1:])
			}
			source.WriteString("\n")
			end++
		}
		if iterator, err := lexer.Tokenise(nil, source.String()); err == nil {
			for offset, tokens := range chroma.SplitTokensIntoLines(iterator.Tokens()) {
				if start+offset < end {
					file.lines[start+offset].code = codeSegments(tokens)
				}
			}
		}
		start = end
	}
}

// pairLines lays the file out for the side-by-side view: each run of removed lines sits
// beside the added lines that replace it.
func (file *diffFile) pairLines() {
	lines := file.lines
	highestNumber := 0
	for index := 0; index < len(lines); {
		highestNumber = max(highestNumber, lines[index].oldNumber, lines[index].newNumber)
		if lines[index].kind != removedLine && lines[index].kind != addedLine {
			file.splitRows = append(file.splitRows, splitRow{index, index})
			index++
			continue
		}
		removedStart := index
		for index < len(lines) && lines[index].kind == removedLine {
			index++
		}
		addedStart := index
		for index < len(lines) && lines[index].kind == addedLine {
			highestNumber = max(highestNumber, lines[index].newNumber)
			index++
		}
		removed, added := addedStart-removedStart, index-addedStart
		for offset := range max(removed, added) {
			row := splitRow{-1, -1}
			if offset < removed {
				row.left = removedStart + offset
			}
			if offset < added {
				row.right = addedStart + offset
			}
			file.splitRows = append(file.splitRows, row)
		}
	}
	file.numberWidth = len(strconv.Itoa(highestNumber))
}

func (file diffFile) renderSplitRow(row splitRow, width int, separator string) string {
	if row.left == row.right && file.lines[row.left].kind < contextLine {
		return file.lines[row.left].render(width)
	}
	side := (width - 3) / 2
	return file.renderSide(row.left, side, true) + separator + file.renderSide(row.right, side, false)
}

func (file diffFile) renderSide(index, width int, isOldSide bool) string {
	if index < 0 {
		return strings.Repeat(" ", width)
	}
	line := file.lines[index]
	number := line.newNumber
	if isOldSide {
		number = line.oldNumber
	}
	codeWidth := max(width-file.numberWidth-1, 1)
	return ui.DimStyle.Render(fmt.Sprintf("%*d ", file.numberWidth, number)) + ui.Fit(line.render(codeWidth), codeWidth)
}

func codeSegments(tokens []chroma.Token) []ui.Segment {
	segments := []ui.Segment{}
	for _, token := range tokens {
		text := strings.TrimRight(token.Value, "\n")
		if text == "" {
			continue
		}
		style := ui.PlainStyle
		entry := syntaxTheme.Get(token.Type)
		if entry.Colour.IsSet() {
			style = style.Foreground(lipgloss.Color(entry.Colour.String()))
		}
		if entry.Bold == chroma.Yes {
			style = style.Bold(true)
		}
		segments = append(segments, ui.Segment{Text: text, Style: style})
	}
	return segments
}

func (line diffLine) render(width int) string {
	switch line.kind {
	case metadataLine:
		return ui.DimStyle.Render(ansi.Truncate(line.text, width, "…"))
	case hunkLine:
		return ui.CyanStyle.Render(ansi.Truncate(line.text, width, "…"))
	}
	marker, code := "", line.code
	if line.text != "" {
		marker = line.text[:1]
		if code == nil {
			code = []ui.Segment{{Text: line.text[1:], Style: ui.PlainStyle}}
		}
	}
	if line.kind == contextLine {
		return ansi.Truncate(marker+ui.StyledText(code), width, "…")
	}
	markerStyle, background := ui.GreenStyle, ui.AddedBackground
	if line.kind == removedLine {
		markerStyle, background = ui.RedStyle, ui.RemovedBackground
	}
	var rendered strings.Builder
	rendered.WriteString(markerStyle.Background(background).Render(marker))
	for _, part := range code {
		rendered.WriteString(part.Style.Background(background).Render(part.Text))
	}
	text := ansi.Truncate(rendered.String(), width, "…")
	padding := strings.Repeat(" ", max(0, width-lipgloss.Width(text)))
	return text + lipgloss.NewStyle().Background(background).Render(padding)
}

func (file diffFile) listSegments(width int) []ui.Segment {
	counts := lineCountSegments(file.additions, file.deletions)
	path := []rune(file.path)
	available := max(width-lipgloss.Width(ui.PlainText(counts))-1, 1)
	if len(path) > available {
		path = append([]rune("…"), path[len(path)-available+1:]...)
	}
	return append(counts, ui.Segment{Text: " " + string(path), Style: ui.PlainStyle})
}
