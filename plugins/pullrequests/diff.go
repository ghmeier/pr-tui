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
	path      string
	additions int
	deletions int
	lines     []diffLine
	// oneSided is a file that was added or deleted outright, so half of a side-by-side view
	// would be empty.
	oneSided    bool
	splitRows   []splitRow
	numberWidth int
}

var hunkHeaderPattern = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)`)

// splitSeparator sits between the two sides of a side-by-side row.
var splitSeparator = ui.DimStyle.Render(" │ ")

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
			if !inHunk && (strings.HasPrefix(line, "new file mode") || strings.HasPrefix(line, "deleted file mode")) {
				file.oneSided = true
			}
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

// splitSideWidth is the width of each side of a side-by-side row, which has a three-cell
// separator between the sides.
func splitSideWidth(width int) int {
	return (width - 3) / 2
}

func (file diffFile) sideCodeWidth(sideWidth int) int {
	return max(sideWidth-file.numberWidth-1, 1)
}

func (row splitRow) spansBothSides(file diffFile) bool {
	return row.left == row.right && file.lines[row.left].kind < contextLine
}

// splitRowHeight is how many terminal lines the row takes once its long lines wrap.
func (file diffFile) splitRowHeight(row splitRow, width int) int {
	if row.spansBothSides(file) {
		return len(file.lines[row.left].parts(width))
	}
	codeWidth := file.sideCodeWidth(splitSideWidth(width))
	height := 1
	for _, index := range []int{row.left, row.right} {
		if index >= 0 {
			height = max(height, len(file.lines[index].parts(codeWidth)))
		}
	}
	return height
}

// renderSplitRow draws the row as one string per terminal line. The first line has the given
// separator between the sides, and the lines that a long line wraps onto have the plain one.
func (file diffFile) renderSplitRow(row splitRow, width int, firstSeparator string) []string {
	if row.spansBothSides(file) {
		return file.lines[row.left].renderLines(width)
	}
	sideWidth := splitSideWidth(width)
	left, right := file.renderSide(row.left, sideWidth, true), file.renderSide(row.right, sideWidth, false)
	blank := strings.Repeat(" ", sideWidth)
	sideAt := func(side []string, offset int) string {
		if offset < len(side) {
			return side[offset]
		}
		return blank
	}
	lines := make([]string, max(len(left), len(right), 1))
	for offset := range lines {
		separator := splitSeparator
		if offset == 0 {
			separator = firstSeparator
		}
		lines[offset] = sideAt(left, offset) + separator + sideAt(right, offset)
	}
	return lines
}

// renderSide shows the line number beside the first line of the code only, so a wrapped
// line is not mistaken for several.
func (file diffFile) renderSide(index, width int, isOldSide bool) []string {
	if index < 0 {
		return nil
	}
	line := file.lines[index]
	number := line.newNumber
	if isOldSide {
		number = line.oldNumber
	}
	codeWidth := file.sideCodeWidth(width)
	lines := line.renderLines(codeWidth)
	for offset, text := range lines {
		gutter := strings.Repeat(" ", file.numberWidth+1)
		if offset == 0 {
			gutter = ui.DimStyle.Render(fmt.Sprintf("%*d ", file.numberWidth, number))
		}
		lines[offset] = gutter + ui.Fit(text, codeWidth)
	}
	return lines
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

// parts splits the line into the pieces that each take one terminal line. For a line of
// code the width includes the column of the +, - or space marker.
func (line diffLine) parts(width int) [][]ui.Segment {
	switch line.kind {
	case metadataLine:
		return ui.WrapSegments([]ui.Segment{{Text: line.text, Style: ui.DimStyle}}, width)
	case hunkLine:
		return ui.WrapSegments([]ui.Segment{{Text: line.text, Style: ui.CyanStyle}}, width)
	}
	code := line.code
	if line.text == "" {
		code = nil
	} else if code == nil {
		code = ui.PlainSegments(line.text[1:])
	}
	return ui.WrapSegments(code, width-1)
}

// renderLines draws the line wrapped to the width. A line that was added or removed keeps its
// background across every wrapped part, up to the full width.
func (line diffLine) renderLines(width int) []string {
	parts := line.parts(width)
	rendered := make([]string, len(parts))
	for offset, part := range parts {
		marker := " "
		if offset == 0 {
			marker = ""
			if line.text != "" {
				marker = line.text[:1]
			}
		}
		switch line.kind {
		case metadataLine, hunkLine:
			rendered[offset] = ui.StyledText(part)
			continue
		case contextLine:
			rendered[offset] = marker + ui.StyledText(part)
			continue
		}
		markerStyle, background := ui.GreenStyle, ui.AddedBackground
		if line.kind == removedLine {
			markerStyle, background = ui.RedStyle, ui.RemovedBackground
		}
		var text strings.Builder
		text.WriteString(markerStyle.Background(background).Render(marker))
		for _, segment := range part {
			text.WriteString(segment.Style.Background(background).Render(segment.Text))
		}
		padding := strings.Repeat(" ", max(0, width-lipgloss.Width(text.String())))
		rendered[offset] = text.String() + lipgloss.NewStyle().Background(background).Render(padding)
	}
	return rendered
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
