package workflows

import (
	"fmt"
	"time"
)

// durationText keeps seconds only while they matter: "45s", "1m32s", "12m", "1h04m". Nothing
// is shown for a duration that is zero.
func durationText(duration time.Duration) string {
	seconds := int(duration.Round(time.Second).Seconds())
	switch {
	case seconds <= 0:
		return ""
	case seconds < 60:
		return fmt.Sprintf("%ds", seconds)
	case seconds < 10*60:
		return fmt.Sprintf("%dm%02ds", seconds/60, seconds%60)
	case seconds < 60*60:
		return fmt.Sprintf("%dm", seconds/60)
	}
	return fmt.Sprintf("%dh%02dm", seconds/3600, seconds%3600/60)
}
