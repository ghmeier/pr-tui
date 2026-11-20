package plugin

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ghmeier/pr-tui/internal/plugin/plugintest"
)

const quickInterval = 30 * time.Second

// startedTimer is one refresh timer the host asked for.
type startedTimer struct {
	board      int
	generation int
	delay      time.Duration
}

// refreshHarness replaces the host's timers and clock, so that a test decides when time passes.
type refreshHarness struct {
	model   *Model
	started []startedTimer
	clock   time.Time
}

func (harness *refreshHarness) advance(duration time.Duration) {
	harness.clock = harness.clock.Add(duration)
}

// latestTimer is the newest timer started for the board.
func (harness *refreshHarness) latestTimer(t *testing.T, board int) startedTimer {
	t.Helper()
	for index := len(harness.started) - 1; index >= 0; index-- {
		if harness.started[index].board == board {
			return harness.started[index]
		}
	}
	t.Fatalf("no timer was started for board %d, timers = %+v", board, harness.started)
	return startedTimer{}
}

func (harness *refreshHarness) timersFor(board int) []startedTimer {
	var timers []startedTimer
	for _, timer := range harness.started {
		if timer.board == board {
			timers = append(timers, timer)
		}
	}
	return timers
}

// tick delivers a timer going off.
func (harness *refreshHarness) tick(t *testing.T, timer startedTimer) {
	t.Helper()
	plugintest.Send(t, harness.model, refreshTickMessage{board: timer.board, generation: timer.generation})
}

// startedModel runs the program's start on the plugins' boards with fake timers and a clock that
// only moves when the test says so.
func startedModel(t *testing.T, plugins ...Plugin) *refreshHarness {
	t.Helper()
	names := make([]string, len(plugins))
	for index, plugin := range plugins {
		names[index] = `"` + plugin.Name() + `"`
	}
	host, err := buildHost(t, "plugins = ["+strings.Join(names, ", ")+"]\nrefresh_interval = \"3m\"\n", plugins...)
	if err != nil {
		t.Fatal(err)
	}
	harness := &refreshHarness{model: host.Model(), clock: time.Date(2026, 1, 2, 9, 0, 0, 0, time.UTC)}
	harness.model.now = func() time.Time { return harness.clock }
	harness.model.startTimer = func(board, generation int, delay time.Duration) tea.Cmd {
		harness.started = append(harness.started, startedTimer{board: board, generation: generation, delay: delay})
		return nil
	}
	plugintest.Settle(t, harness.model, harness.model.Init())
	return harness
}

func quickBoard(name string, source *rowSource, busy func(rows []Row) bool) Board {
	board := sectionBoard(name, name, source)
	board.RefreshInterval = quickInterval
	board.Busy = busy
	return board
}

func always(rows []Row) bool { return true }
func never(rows []Row) bool  { return false }

func TestBoardSchedulesItsNextLoadOnlyAfterItsLoadFinishes(t *testing.T) {
	harness := startedModel(t, boardPlugin("shop", groceryBoard(newRowSource(groceries()...))))
	startedAfterFirstLoad := len(harness.started)
	harness.advance(harness.model.refreshInterval)

	harness.tick(t, harness.latestTimer(t, 0))

	if startedAfterFirstLoad != 1 || len(harness.started) != 2 {
		t.Fatalf("timers after the first load = %d, after the second = %d", startedAfterFirstLoad, len(harness.started))
	}
}

func TestInitStartsNoTimerBeforeAnyLoadFinishes(t *testing.T) {
	host, err := buildHost(t, `plugins = ["fake"]`, boardPlugin("fake", groceryBoard(newRowSource(groceries()...))))
	if err != nil {
		t.Fatal(err)
	}
	model := host.Model()
	timers := 0
	model.startTimer = func(int, int, time.Duration) tea.Cmd {
		timers++
		return nil
	}

	model.Init()

	if timers != 0 {
		t.Fatalf("%d timers were started before a load finished", timers)
	}
}

func TestEachBoardHasItsOwnTimer(t *testing.T) {
	first, second := newRowSource(groceries()...), newRowSource(groceries()...)
	harness := startedModel(t, boardsPlugin(sectionBoard("fresh", "Fresh", first), sectionBoard("frozen", "Frozen", second)))
	harness.advance(harness.model.refreshInterval)
	secondBefore := second.loadCount()
	firstBefore := first.loadCount()

	harness.tick(t, harness.latestTimer(t, 0))

	if first.loadCount() != firstBefore+1 || second.loadCount() != secondBefore {
		t.Fatalf("loads: first %d more, second %d more", first.loadCount()-firstBefore, second.loadCount()-secondBefore)
	}
	if len(harness.timersFor(0)) != 2 || len(harness.timersFor(1)) != 1 {
		t.Fatalf("timers = %+v", harness.started)
	}
}

func TestBusyBoardRefreshesAtItsOwnInterval(t *testing.T) {
	harness := startedModel(t, boardsPlugin(quickBoard("quick", newRowSource(groceries()...), always)))

	if timer := harness.latestTimer(t, 0); timer.delay != quickInterval {
		t.Fatalf("delay = %v, expected %v", timer.delay, quickInterval)
	}
}

func TestBoardWhoseBusyReportsFalseRefreshesAtTheConfiguredInterval(t *testing.T) {
	harness := startedModel(t, boardsPlugin(quickBoard("quiet", newRowSource(groceries()...), never)))

	if timer := harness.latestTimer(t, 0); timer.delay != harness.model.refreshInterval {
		t.Fatalf("delay = %v, expected %v", timer.delay, harness.model.refreshInterval)
	}
}

func TestBoardWithoutBusyAlwaysRefreshesAtItsOwnInterval(t *testing.T) {
	harness := startedModel(t, boardsPlugin(quickBoard("steady", newRowSource(groceries()...), nil)))

	if timer := harness.latestTimer(t, 0); timer.delay != quickInterval {
		t.Fatalf("delay = %v, expected %v", timer.delay, quickInterval)
	}
}

func TestBusyIsAskedAboutTheRowsTheBoardLoaded(t *testing.T) {
	source := newRowSource(fruit("busy-key", "busy"))
	busyRows := func(rows []Row) bool { return len(rows) == 1 && rows[0].Key() == "busy-key" }
	harness := startedModel(t, boardsPlugin(quickBoard("quick", source, busyRows)))
	source.set(groceries()...)
	harness.advance(harness.model.refreshInterval)

	harness.tick(t, harness.latestTimer(t, 0))

	if timer := harness.latestTimer(t, 0); timer.delay != harness.model.refreshInterval {
		t.Fatalf("delay = %v, expected %v once the rows were no longer busy", timer.delay, harness.model.refreshInterval)
	}
}

func TestTickOfAReplacedTimerDoesNotLoadTheBoard(t *testing.T) {
	source := newRowSource(groceries()...)
	harness := startedModel(t, boardPlugin("shop", groceryBoard(source)))
	oldTimer := harness.latestTimer(t, 0)
	plugintest.Press(t, harness.model, "r")
	harness.advance(harness.model.refreshInterval)
	before := source.loadCount()

	harness.tick(t, oldTimer)

	if source.loadCount() != before {
		t.Fatalf("a stale tick loaded the board")
	}
}

func TestTickOfTheLatestTimerLoadsTheBoard(t *testing.T) {
	source := newRowSource(groceries()...)
	harness := startedModel(t, boardPlugin("shop", groceryBoard(source)))
	plugintest.Press(t, harness.model, "r")
	harness.advance(harness.model.refreshInterval)
	before := source.loadCount()

	harness.tick(t, harness.latestTimer(t, 0))

	if source.loadCount() != before+1 {
		t.Fatalf("loads = %d, expected %d", source.loadCount(), before+1)
	}
}

func TestTickWhileTheBoardIsLoadingDoesNotStartAnotherLoad(t *testing.T) {
	source := newRowSource(groceries()...)
	harness := startedModel(t, boardPlugin("shop", groceryBoard(source)))
	timer := harness.latestTimer(t, 0)
	harness.model.update(plugintest.KeyMessage("r"))
	harness.advance(harness.model.refreshInterval)
	before := source.loadCount()

	harness.tick(t, timer)

	if source.loadCount() != before {
		t.Fatalf("loads = %d, expected %d while a load was in flight", source.loadCount(), before)
	}
}

func TestTickBeforeTheBoardsDataIsOlderThanItsDelayWaitsOutTheRest(t *testing.T) {
	source := newRowSource(groceries()...)
	harness := startedModel(t, boardPlugin("shop", groceryBoard(source)))
	timer := harness.latestTimer(t, 0)
	harness.advance(harness.model.refreshInterval - time.Second)
	before := source.loadCount()

	harness.tick(t, timer)

	if source.loadCount() != before || harness.latestTimer(t, 0).delay != time.Second {
		t.Fatalf("loads = %d more, latest timer = %+v", source.loadCount()-before, harness.latestTimer(t, 0))
	}
}

func TestBoardOfATabThatIsNotShownIsScheduledAtLeastAtTheConfiguredInterval(t *testing.T) {
	harness := startedModel(t, boardPlugin("solo", soloBoard()), boardsPlugin(quickBoard("quick", newRowSource(groceries()...), always)))

	if timer := harness.latestTimer(t, 1); timer.delay != harness.model.refreshInterval {
		t.Fatalf("delay = %v, expected %v while its tab is hidden", timer.delay, harness.model.refreshInterval)
	}
}

func TestBoardOfTheShownTabIsNotHeldBackToTheConfiguredInterval(t *testing.T) {
	harness := startedModel(t, boardsPlugin(quickBoard("quick", newRowSource(groceries()...), always)), boardPlugin("solo", soloBoard()))

	if timer := harness.latestTimer(t, 0); timer.delay != quickInterval {
		t.Fatalf("delay = %v, expected %v", timer.delay, quickInterval)
	}
}

func TestShowingATabLoadsItsBoardsThatAreOlderThanTheirDelayAndNotTheFreshOnes(t *testing.T) {
	stale, fresh := newRowSource(groceries()...), newRowSource(groceries()...)
	harness := startedModel(t, boardsPlugin(
		sectionBoard("stale", "Stale", stale),
		sectionBoard("fresh", "Fresh", fresh),
	), boardPlugin("solo", soloBoard()))
	plugintest.Press(t, harness.model, "tab")
	harness.advance(harness.model.refreshInterval)
	plugintest.Send(t, harness.model, loadedMessage{board: 1, generation: harness.model.boards[1].loadGeneration, rows: groceries()})
	staleBefore, freshBefore := stale.loadCount(), fresh.loadCount()

	plugintest.Press(t, harness.model, "tab")

	if stale.loadCount() != staleBefore+1 || fresh.loadCount() != freshBefore {
		t.Fatalf("loads: stale %d more, fresh %d more", stale.loadCount()-staleBefore, fresh.loadCount()-freshBefore)
	}
}

func TestShowingATabReschedulesAFreshBoardForTheRestOfItsDelay(t *testing.T) {
	harness := startedModel(t, boardsPlugin(quickBoard("quick", newRowSource(groceries()...), always)), boardPlugin("solo", soloBoard()))
	plugintest.Press(t, harness.model, "tab")
	harness.advance(10 * time.Second)

	plugintest.Press(t, harness.model, "tab")

	if timer := harness.latestTimer(t, 0); timer.delay != quickInterval-10*time.Second {
		t.Fatalf("delay = %v, expected %v", timer.delay, quickInterval-10*time.Second)
	}
}

func TestStatusLineShowsTheCurrentDelayOfABusyAndAQuietBoard(t *testing.T) {
	busy := startedModel(t, boardsPlugin(quickBoard("quick", newRowSource(groceries()...), always)))
	quiet := startedModel(t, boardsPlugin(quickBoard("quick", newRowSource(groceries()...), never)))

	if status := statusLine(busy.model); !strings.Contains(status, "refreshes every "+intervalText(quickInterval)) {
		t.Fatalf("busy status line = %q", status)
	}
	if status := statusLine(quiet.model); !strings.Contains(status, "refreshes every "+intervalText(quiet.model.refreshInterval)) {
		t.Fatalf("quiet status line = %q", status)
	}
}
