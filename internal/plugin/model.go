package plugin

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ghmeier/pr-tui/internal/ui"
)

type mode int

const (
	boardMode mode = iota
	filterMode
	confirmMode
)

type (
	loadedMessage struct {
		board int
		rows  []Row
		err   error
	}
	columnLoadedMessage struct {
		board  int
		column int
		values map[string]any
		err    error
	}
	commandFinishedMessage struct {
		board   int
		err     error
		refresh bool
	}
	refreshTickMessage struct{}
)

// board is a registered board and everything the host keeps about it while it is shown.
type board struct {
	spec    Board
	rows    []Row
	visible []Row
	// data holds, for each column, the values its fetches delivered, by row key.
	data       []map[string]any
	facets     []facetState
	menuCursor int
	loadedAt   time.Time
	cursor     int
	offset     int
	notice     string
}

// Model is the root Bubble Tea model: the boards with their tabs, and the screens above them.
type Model struct {
	boards          []*board
	active          int
	screens         []Screen
	refreshInterval time.Duration
	// startTimer starts the timer for the next scheduled refresh. Tests replace it, because a
	// real timer would make them wait.
	startTimer func(interval time.Duration) tea.Cmd
	width      int
	height     int
	mode       mode
	question   string
	// pending is the effect waiting for an answer to its question.
	pending Effect
}

// Model builds the interactive program for the boards the plugins registered.
func (host *Host) Model() *Model {
	model := &Model{refreshInterval: host.RefreshInterval(), startTimer: startRefreshTimer, width: 120, height: 30}
	for _, spec := range host.boards {
		next := &board{spec: spec, data: make([]map[string]any, len(spec.Columns))}
		for index := range next.data {
			next.data[index] = map[string]any{}
		}
		for _, facet := range spec.Facets {
			next.facets = append(next.facets, newFacetState(facet))
		}
		model.boards = append(model.boards, next)
	}
	return model
}

func (m *Model) current() *board {
	return m.boards[m.active]
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.RefreshAll(), m.scheduleRefresh())
}

// RefreshAll reloads every board.
func (m *Model) RefreshAll() tea.Cmd {
	commands := make([]tea.Cmd, len(m.boards))
	for index := range m.boards {
		commands[index] = m.load(index)
	}
	return tea.Batch(commands...)
}

func startRefreshTimer(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(time.Time) tea.Msg { return refreshTickMessage{} })
}

func (m *Model) scheduleRefresh() tea.Cmd {
	return m.startTimer(m.refreshInterval)
}

func (m *Model) load(index int) tea.Cmd {
	load := m.boards[index].spec.Load
	return func() tea.Msg {
		rows, err := load()
		return loadedMessage{board: index, rows: rows, err: err}
	}
}

// fetchCommands starts every column fetch of a board. The rows given to a column are a copy,
// because the fetches run while the board goes on changing.
func (m *Model) fetchCommands(index int) []tea.Cmd {
	b := m.boards[index]
	var commands []tea.Cmd
	for columnIndex, column := range b.spec.Columns {
		if column.Load == nil {
			continue
		}
		for _, fetch := range column.Load(slices.Clone(b.rows)) {
			commands = append(commands, func() tea.Msg {
				values, err := fetch()
				return columnLoadedMessage{board: index, column: columnIndex, values: values, err: err}
			})
		}
	}
	return commands
}

// finishLoad shows freshly loaded rows and returns the commands that fetch their column data.
func (m *Model) finishLoad(index int, rows []Row) []tea.Cmd {
	b := m.boards[index]
	b.notice = ""
	b.rows = rows
	b.loadedAt = time.Now()
	// Values for rows that are gone would only pile up.
	keys := map[string]bool{}
	for _, row := range rows {
		keys[row.Key()] = true
	}
	for _, values := range b.data {
		for key := range values {
			if !keys[key] {
				delete(values, key)
			}
		}
	}
	b.applyFilters()
	return m.fetchCommands(index)
}

func (b *board) selected() (Row, bool) {
	if b.cursor < 0 || b.cursor >= len(b.visible) {
		return nil, false
	}
	return b.visible[b.cursor], true
}

// applyFilters recomputes the visible rows and keeps the cursor on the same row.
func (b *board) applyFilters() {
	selected, hadSelection := b.selected()
	b.visible = nil
	for _, row := range b.rows {
		if b.matches(row) {
			b.visible = append(b.visible, row)
		}
	}
	if !hadSelection {
		return
	}
	for index, row := range b.visible {
		if row.Key() == selected.Key() {
			b.cursor = index
			return
		}
	}
}

func (b *board) matches(row Row) bool {
	for _, state := range b.facets {
		if !state.matches(row) {
			return false
		}
	}
	return true
}

func (b *board) selection() (Selection, bool) {
	row, hasSelection := b.selected()
	if !hasSelection {
		return Selection{}, false
	}
	return Selection{Row: row, Data: func(header string) any {
		index := slices.IndexFunc(b.spec.Columns, func(column Column) bool { return column.Header == header })
		if index < 0 {
			return nil
		}
		return b.data[index][row.Key()]
	}}, true
}

func (b *board) cells() [][][]ui.Segment {
	cells := make([][][]ui.Segment, len(b.visible))
	for rowIndex, row := range b.visible {
		cells[rowIndex] = make([][]ui.Segment, len(b.spec.Columns))
		for columnIndex, column := range b.spec.Columns {
			cells[rowIndex][columnIndex] = column.Cell(row, b.data[columnIndex][row.Key()])
		}
	}
	return cells
}

func (b *board) table(width, selected, offset, rowCount int) string {
	headers := make([]string, len(b.spec.Columns))
	flexible := -1
	for index, column := range b.spec.Columns {
		headers[index] = column.Header
		if column.Flexible {
			flexible = index
		}
	}
	return renderTable(headers, flexible, b.cells(), width, selected, offset, rowCount)
}

func (m *Model) tableRows() int {
	rows := m.height - 4
	if len(m.boards) > 1 {
		rows--
	}
	if m.mode == filterMode {
		rows -= lipgloss.Height(filterMenu(m.current().facets, m.current().menuCursor))
	}
	return max(rows, 1)
}

func (m *Model) keepCursorVisible() {
	for _, b := range m.boards {
		b.cursor = max(0, min(b.cursor, len(b.visible)-1))
		b.offset = min(b.offset, b.cursor)
		b.offset = max(b.offset, b.cursor-m.tableRows()+1)
	}
}

func (m *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	command := m.update(message)
	m.keepCursorVisible()
	return m, command
}

func (m *Model) update(message tea.Msg) tea.Cmd {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = message.Width, message.Height
	case openScreenMessage:
		m.screens = append(m.screens, message.screen)
		return message.screen.Init()
	case noticeMessage:
		m.current().notice = message.text
	case loadedMessage:
		b := m.boards[message.board]
		if message.err != nil {
			b.notice = "Refresh failed: " + ui.FirstLine(message.err)
			return nil
		}
		return tea.Batch(m.finishLoad(message.board, message.rows)...)
	case columnLoadedMessage:
		if message.err != nil {
			return nil
		}
		b := m.boards[message.board]
		for key, value := range message.values {
			b.data[message.column][key] = value
		}
		b.applyFilters()
	case commandFinishedMessage:
		b := m.boards[message.board]
		switch {
		case message.err != nil:
			b.notice = "Failed: " + ui.FirstLine(message.err)
		case message.refresh:
			return m.load(message.board)
		default:
			b.notice = ""
		}
	case refreshTickMessage:
		return tea.Batch(m.RefreshAll(), m.scheduleRefresh())
	case tea.KeyMsg:
		if message.String() == "ctrl+c" {
			return tea.Quit
		}
		if len(m.screens) > 0 {
			return m.updateTopScreen(message)
		}
		return m.handleKey(message.String())
	default:
		return m.updateEveryScreen(message)
	}
	return nil
}

func (m *Model) updateTopScreen(message tea.Msg) tea.Cmd {
	top := len(m.screens) - 1
	next, command := m.screens[top].Update(message)
	if next == nil {
		m.screens = m.screens[:top]
	} else {
		m.screens[top] = next
	}
	return command
}

// updateEveryScreen delivers a message to each open screen, topmost first, because a message
// from a screen's background command may arrive after other screens were opened above it.
func (m *Model) updateEveryScreen(message tea.Msg) tea.Cmd {
	var commands []tea.Cmd
	for index := len(m.screens) - 1; index >= 0; index-- {
		next, command := m.screens[index].Update(message)
		commands = append(commands, command)
		if next == nil {
			m.screens = slices.Delete(m.screens, index, index+1)
		} else {
			m.screens[index] = next
		}
	}
	return tea.Batch(commands...)
}

func (m *Model) handleKey(key string) tea.Cmd {
	if key == "q" {
		return tea.Quit
	}
	switch m.mode {
	case filterMode:
		m.handleFilterMenuKey(key)
		return nil
	case confirmMode:
		return m.handleConfirmKey(key)
	}

	b := m.current()
	switch key {
	case "up", "k":
		b.cursor--
	case "down", "j":
		b.cursor++
	case "pgup":
		b.cursor -= m.tableRows()
	case "pgdown":
		b.cursor += m.tableRows()
	case "home":
		b.cursor = 0
	case "end":
		b.cursor = len(b.visible) - 1
	case "f":
		if len(b.facets) > 0 {
			m.mode = filterMode
		}
	case "r":
		b.notice = "Refreshing…"
		return m.load(m.active)
	case "tab":
		m.active = (m.active + 1) % len(m.boards)
	case "shift+tab":
		m.active = (m.active + len(m.boards) - 1) % len(m.boards)
	default:
		return m.runAction(key)
	}
	return nil
}

func (m *Model) runAction(key string) tea.Cmd {
	b := m.current()
	selection, hasSelection := b.selection()
	if !hasSelection {
		return nil
	}
	for _, action := range b.spec.Actions {
		if action.Key != key {
			continue
		}
		effect := action.Run(selection)
		if effect.Confirm != "" {
			m.mode, m.question, m.pending = confirmMode, effect.Confirm, effect
			return nil
		}
		return m.execute(m.active, effect)
	}
	return nil
}

// execute carries out an effect, with its confirmation already answered.
func (m *Model) execute(index int, effect Effect) tea.Cmd {
	var commands []tea.Cmd
	if effect.Notice != "" {
		m.boards[index].notice = effect.Notice
	}
	if effect.Command != nil {
		run := effect.Command
		commands = append(commands, func() tea.Msg {
			return commandFinishedMessage{board: index, err: run(), refresh: effect.Refresh}
		})
	}
	if effect.Screen != nil {
		m.screens = append(m.screens, effect.Screen)
		commands = append(commands, effect.Screen.Init())
	}
	return tea.Batch(commands...)
}

func (m *Model) handleConfirmKey(key string) tea.Cmd {
	switch key {
	case "y", "enter":
		effect := m.pending
		m.mode, m.question, m.pending = boardMode, "", Effect{}
		if effect.Notice == "" {
			effect.Notice = "Working…"
		}
		return m.execute(m.active, effect)
	case "n", "esc":
		m.mode, m.question, m.pending = boardMode, "", Effect{}
	}
	return nil
}

func (m *Model) handleFilterMenuKey(key string) {
	b := m.current()
	switch key {
	case "f", "esc", "enter":
		m.mode = boardMode
		return
	}
	if handleFilterKey(b.facets, &b.menuCursor, key) {
		b.applyFilters()
	}
}

func (m *Model) statusLine() string {
	b := m.current()
	switch {
	case m.mode == confirmMode:
		return ui.KeyStyle.Render(m.question) + "  " + ui.HelpLine("y", "yes", "n", "no")
	case b.notice != "":
		return b.notice
	case b.loadedAt.IsZero():
		return ui.DimStyle.Render("Loading…")
	}
	return ui.DimStyle.Render(fmt.Sprintf(
		"%d of %d %s · updated %s · refreshes every %s",
		len(b.visible), len(b.rows), b.spec.RowNoun, b.loadedAt.Format("15:04"), intervalText(m.refreshInterval),
	))
}

// intervalText says a refresh interval in whole minutes where it can, as in "3 minutes". A
// single minute or second has no number, so the status line reads "every second".
func intervalText(interval time.Duration) string {
	switch {
	case interval == time.Minute:
		return "minute"
	case interval == time.Second:
		return "second"
	case interval%time.Minute == 0:
		return fmt.Sprintf("%d minutes", interval/time.Minute)
	}
	return fmt.Sprintf("%d seconds", int(interval.Round(time.Second)/time.Second))
}

func (m *Model) help() string {
	switch m.mode {
	case filterMode:
		return ui.HelpLine("esc", "close filters", "q", "quit")
	case confirmMode:
		return ""
	}
	var keysAndLabels []string
	for _, action := range m.current().spec.Actions {
		if action.Label != "" {
			keysAndLabels = append(keysAndLabels, action.Key, action.Label)
		}
	}
	if len(m.current().facets) > 0 {
		keysAndLabels = append(keysAndLabels, "f", "filters")
	}
	return ui.HelpLine(append(keysAndLabels, "r", "refresh", "q", "quit")...)
}

func (m *Model) tabBar() string {
	tabs := make([]string, len(m.boards))
	for index, b := range m.boards {
		if index == m.active {
			tabs[index] = ui.SelectedStyle.Render(" " + b.spec.Title + " ")
		} else {
			tabs[index] = ui.DimStyle.Render(" " + b.spec.Title + " ")
		}
	}
	return strings.Join(tabs, " ") + "  " + ui.HelpLine("tab", "next")
}

func (m *Model) View() string {
	if len(m.screens) > 0 {
		return m.screens[len(m.screens)-1].View(m.width, m.height)
	}
	b := m.current()
	var sections []string
	if len(m.boards) > 1 {
		sections = append(sections, ansi.Truncate(m.tabBar(), m.width, "…"))
	}
	sections = append(sections, facetSummary(b.facets))
	if m.mode == filterMode {
		sections = append(sections, filterMenu(b.facets, b.menuCursor))
	}
	rows := m.tableRows()
	table := b.table(m.width, b.cursor, b.offset, rows)
	blankLines := rows + 1 - lipgloss.Height(table)
	sections = append(sections, table+strings.Repeat("\n", max(blankLines, 0)))
	sections = append(sections, ansi.Truncate(m.statusLine(), m.width, "…"), ansi.Truncate(m.help(), m.width, "…"))
	return strings.Join(sections, "\n")
}
