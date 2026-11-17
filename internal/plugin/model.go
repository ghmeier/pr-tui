package plugin

import (
	"slices"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/ui"
)

type mode int

const (
	boardMode mode = iota
	filterMode
	confirmMode
	choiceMode
)

type (
	// The generation of a load orders it among the loads of its board, so that a result that
	// arrives after a newer one can be recognised and dropped.
	loadedMessage struct {
		board      int
		generation int
		rows       []Row
		err        error
	}
	columnLoadedMessage struct {
		board      int
		generation int
		column     int
		values     map[string]any
		err        error
	}
	// effectMessage asks the host to carry out an effect for a board, as an action's effect
	// would be.
	effectMessage struct {
		board  int
		effect Effect
	}
	// actionMessage asks the host to run a board's action as if its key was pressed on the row.
	actionMessage struct {
		board    int
		rowKey   string
		fallback Row
		key      string
	}
	commandFinishedMessage struct {
		board   int
		err     error
		refresh bool
	}
	refreshTickMessage struct{}
)

// Model is the root Bubble Tea model: the boards with their tabs, and the screens above them.
type Model struct {
	boards  []*boardState
	active  int
	screens []Screen
	// screenBoards is the board each open screen was opened for, parallel to screens.
	screenBoards    []int
	refreshInterval time.Duration
	// startTimer starts the timer for the next scheduled refresh. Tests replace it, because a
	// real timer would make them wait.
	startTimer func(interval time.Duration) tea.Cmd
	width      int
	height     int
	mode       mode
	question   string
	// pending is the effect waiting for an answer to its question, and pendingBoard the board
	// it applies to, whether the question is a confirmation or a choice.
	pending      Effect
	pendingBoard int
	choice       *Choice
}

// Model builds the interactive program for the boards the plugins registered.
func (host *Host) Model() *Model {
	model := &Model{refreshInterval: host.file.RefreshInterval, startTimer: startRefreshTimer, width: 120, height: 30}
	for index, spec := range host.boards {
		next := &boardState{
			spec:            spec,
			index:           index,
			data:            make([]map[string]any, len(spec.Columns)),
			dataGenerations: make([]map[string]int, len(spec.Columns)),
		}
		for column := range next.data {
			next.data[column] = map[string]any{}
			next.dataGenerations[column] = map[string]int{}
		}
		for _, facet := range spec.Facets {
			next.facets = append(next.facets, newFacetState(facet))
		}
		model.boards = append(model.boards, next)
	}
	return model
}

func (m *Model) current() *boardState {
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
	board := m.boards[index]
	board.loadGeneration++
	generation := board.loadGeneration
	load := board.spec.Load
	return func() tea.Msg {
		rows, err := load()
		return loadedMessage{board: index, generation: generation, rows: rows, err: err}
	}
}

// fetchCommands starts every column fetch of a board for the rows that the given load
// produced. The rows given to a column are a copy, because the fetches run while the board
// goes on changing.
func (m *Model) fetchCommands(index, generation int) []tea.Cmd {
	board := m.boards[index]
	var commands []tea.Cmd
	for columnIndex, column := range board.spec.Columns {
		if column.Load == nil {
			continue
		}
		for _, fetch := range column.Load(slices.Clone(board.rows)) {
			commands = append(commands, func() tea.Msg {
				values, err := fetch()
				return columnLoadedMessage{board: index, generation: generation, column: columnIndex, values: values, err: err}
			})
		}
	}
	return commands
}

// finishLoad shows freshly loaded rows and returns the commands that fetch their column data.
func (m *Model) finishLoad(index int, rows []Row, generation int) []tea.Cmd {
	board := m.boards[index]
	board.notice = ""
	board.rows = rows
	board.rowsGeneration = generation
	board.loadedAt = time.Now()
	// Values for rows that are gone would only pile up.
	keys := map[string]bool{}
	for _, row := range rows {
		keys[row.Key()] = true
	}
	for column, values := range board.data {
		for key := range values {
			if !keys[key] {
				delete(values, key)
				delete(board.dataGenerations[column], key)
			}
		}
	}
	for facet := range board.facets {
		board.facets[facet].refreshOptionsFromRows(rows)
	}
	board.menuCursor = max(0, min(board.menuCursor, optionCount(board.facets)-1))
	board.applyFilters()
	return m.fetchCommands(index, generation)
}

func (m *Model) keepCursorVisible() {
	for _, board := range m.boards {
		board.cursor = max(0, min(board.cursor, len(board.visible)-1))
		board.offset = min(board.offset, board.cursor)
		board.offset = max(board.offset, board.cursor-m.tableRows()+1)
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
	case effectMessage:
		return m.applyEffect(message.board, message.effect)
	case actionMessage:
		board := m.boards[message.board]
		row, found := board.rowWithKey(message.rowKey)
		if !found {
			row = message.fallback
		}
		return m.runBoardAction(message.board, board.selectionOf(row), message.key)
	case loadedMessage:
		board := m.boards[message.board]
		if message.generation < board.rowsGeneration {
			return nil
		}
		if message.err != nil {
			board.notice = "Refresh failed: " + ui.FirstLine(message.err)
			return nil
		}
		return tea.Batch(m.finishLoad(message.board, message.rows, message.generation)...)
	case columnLoadedMessage:
		if message.err != nil {
			return nil
		}
		board := m.boards[message.board]
		generations := board.dataGenerations[message.column]
		for key, value := range message.values {
			if stored, found := generations[key]; found && message.generation < stored {
				continue
			}
			board.data[message.column][key] = value
			generations[key] = message.generation
		}
		board.applyFilters()
	case commandFinishedMessage:
		board := m.boards[message.board]
		switch {
		case message.err != nil:
			board.notice = "Failed: " + ui.FirstLine(message.err)
		case message.refresh:
			return m.load(message.board)
		default:
			board.notice = ""
		}
	case refreshTickMessage:
		return tea.Batch(m.RefreshAll(), m.scheduleRefresh())
	case tea.KeyMsg:
		if message.String() == "ctrl+c" {
			return tea.Quit
		}
		if len(m.screens) > 0 {
			if m.asking() {
				return m.handleAnswerKey(message.String())
			}
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
		m.screenBoards = m.screenBoards[:top]
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
			m.screenBoards = slices.Delete(m.screenBoards, index, index+1)
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
	case confirmMode, choiceMode:
		return m.handleAnswerKey(key)
	}

	board := m.current()
	switch key {
	case "up", "k":
		board.cursor--
	case "down", "j":
		board.cursor++
	case "pgup":
		board.cursor -= m.tableRows()
	case "pgdown":
		board.cursor += m.tableRows()
	case "home":
		board.cursor = 0
	case "end":
		board.cursor = len(board.visible) - 1
	case "f":
		if len(board.facets) > 0 {
			m.mode = filterMode
		}
	case "r":
		board.notice = "Refreshing…"
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
	selection, hasSelection := m.current().selection()
	if !hasSelection {
		return nil
	}
	return m.runBoardAction(m.active, selection, key)
}

// runBoardAction runs the board's action with the key on the selection, if it has one.
func (m *Model) runBoardAction(index int, selection Selection, key string) tea.Cmd {
	for _, action := range m.boards[index].spec.Actions {
		if action.Key == key {
			return m.applyEffect(index, action.Run(selection))
		}
	}
	return nil
}

// asking says whether a confirmation or a choice is waiting for an answer.
func (m *Model) asking() bool {
	return m.mode == confirmMode || m.mode == choiceMode
}

// applyEffect asks the effect's choice or question first, if it has one, and otherwise carries
// it out.
func (m *Model) applyEffect(index int, effect Effect) tea.Cmd {
	if effect.Choice != nil {
		m.mode, m.choice, m.pendingBoard = choiceMode, effect.Choice, index
		return nil
	}
	if effect.Confirm != "" {
		m.mode, m.question, m.pending, m.pendingBoard = confirmMode, effect.Confirm, effect, index
		return nil
	}
	return m.execute(index, effect)
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
		m.screenBoards = append(m.screenBoards, index)
		commands = append(commands, effect.Screen.Init())
	}
	return tea.Batch(commands...)
}

func (m *Model) handleAnswerKey(key string) tea.Cmd {
	if m.mode == choiceMode {
		return m.handleChoiceKey(key)
	}
	return m.handleConfirmKey(key)
}

func (m *Model) handleChoiceKey(key string) tea.Cmd {
	if key == "esc" {
		m.mode, m.choice = boardMode, nil
		return nil
	}
	number, err := strconv.Atoi(key)
	options := m.choice.shownOptions()
	if err != nil || number < 1 || number > len(options) {
		return nil
	}
	choice, index := m.choice, m.pendingBoard
	m.mode, m.choice = boardMode, nil
	return m.applyEffect(index, choice.Pick(options[number-1]))
}

func (m *Model) handleConfirmKey(key string) tea.Cmd {
	switch key {
	case "y", "enter":
		effect, index := m.pending, m.pendingBoard
		m.mode, m.question, m.pending = boardMode, "", Effect{}
		if effect.Notice == "" {
			effect.Notice = "Working…"
		}
		return m.execute(index, effect)
	case "n", "esc":
		m.mode, m.question, m.pending = boardMode, "", Effect{}
	}
	return nil
}

func (m *Model) handleFilterMenuKey(key string) {
	board := m.current()
	switch key {
	case "f", "esc", "enter":
		m.mode = boardMode
		return
	}
	if handleFilterKey(board.facets, &board.menuCursor, key) {
		board.applyFilters()
	}
}
