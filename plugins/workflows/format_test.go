package workflows

import (
	"testing"
	"time"

	"github.com/ghmeier/pr-tui/internal/ui"
)

func TestDurationTextKeepsSecondsOnlyForShortDurations(t *testing.T) {
	cases := map[time.Duration]string{
		0:                               "",
		45 * time.Second:                "45s",
		92 * time.Second:                "1m32s",
		9*time.Minute + 59*time.Second:  "9m59s",
		12 * time.Minute:                "12m",
		12*time.Minute + 40*time.Second: "12m",
		59 * time.Minute:                "59m",
		time.Hour + 4*time.Minute:       "1h04m",
		26*time.Hour + 30*time.Minute + 5*time.Second: "26h30m",
	}

	for duration, expected := range cases {
		if text := durationText(duration); text != expected {
			t.Errorf("durationText(%v) = %q, expected %q", duration, text, expected)
		}
	}
}

func TestRunDurationOfAFinishedRunEndsAtItsLastUpdate(t *testing.T) {
	run := Run{Status: RunStatusSucceeded, StartedAt: at(12, 0), UpdatedAt: at(12, 12)}

	if duration := run.duration(at(18, 0)); duration != 12*time.Minute {
		t.Fatalf("duration = %v", duration)
	}
}

func TestRunDurationOfARunningRunEndsNow(t *testing.T) {
	run := Run{Status: RunStatusRunning, StartedAt: at(12, 0), UpdatedAt: at(12, 1)}

	if duration := run.duration(at(12, 7)); duration != 7*time.Minute {
		t.Fatalf("duration = %v", duration)
	}
}

func TestRunDurationOfARunThatNeverStartedIsZero(t *testing.T) {
	run := Run{Status: RunStatusQueued, UpdatedAt: at(12, 1)}

	if duration := run.duration(at(12, 7)); duration != 0 {
		t.Fatalf("duration = %v", duration)
	}
}

func TestStartedAtFallsBackToTheCreationTimeOfAQueuedRun(t *testing.T) {
	queued := Run{CreatedAt: at(9, 0)}
	started := Run{CreatedAt: at(9, 0), StartedAt: at(9, 5)}

	if queued.startedAt() != queued.CreatedAt || started.startedAt() != started.StartedAt {
		t.Fatalf("queued = %v, started = %v", queued.startedAt(), started.startedAt())
	}
}

func TestEveryStatusHasADistinctMarkAndName(t *testing.T) {
	marks, names := map[string]bool{}, map[string]bool{}
	for _, status := range []RunStatus{RunStatusQueued, RunStatusRunning, RunStatusSucceeded, RunStatusFailed, RunStatusCancelled, RunStatusSkipped} {
		marks[status.mark()], names[status.name()] = true, true
	}

	if len(marks) != 6 || len(names) != 6 {
		t.Fatalf("marks = %v, names = %v", marks, names)
	}
}

func TestAgeTextIsEmptyForAnUnknownTime(t *testing.T) {
	text := ageText(time.Time{})

	if text != "" {
		t.Fatalf("age = %q", text)
	}
}

func TestAgeTextSaysHowLongAgoAKnownTimeWas(t *testing.T) {
	since := time.Now().Add(-4*time.Minute - time.Second)

	text := ageText(since)

	if expected := ui.AgeText(since) + " ago"; text != expected {
		t.Fatalf("age = %q, expected %q", text, expected)
	}
}
