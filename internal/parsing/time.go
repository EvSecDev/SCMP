package parsing

import (
	"fmt"
	"time"
)

// Format elapsed millisecond time to its max unit size plus one smaller unit
// like '17m and 43s' or '29s 532ms'
func FormatElapsedTime(startTime int64, endTime int64) (elapsedWithUnits string) {
	elapsed := endTime - startTime

	// Handle days
	days := elapsed / (1000 * 60 * 60 * 24)
	elapsed %= (1000 * 60 * 60 * 24)

	// Handle hours
	hours := elapsed / (1000 * 60 * 60)
	elapsed %= (1000 * 60 * 60)

	// Handle minutes
	minutes := elapsed / (1000 * 60)
	elapsed %= (1000 * 60)

	// Handle seconds
	seconds := elapsed / 1000
	milliseconds := elapsed % 1000

	// Format based on the largest unit available
	if days > 0 {
		elapsedWithUnits = fmt.Sprintf("%d days and %d hours", days, hours)
	} else if hours > 0 {
		elapsedWithUnits = fmt.Sprintf("%dh and %dm", hours, minutes)
	} else if minutes > 0 {
		elapsedWithUnits = fmt.Sprintf("%dm and %ds", minutes, seconds)
	} else if seconds > 0 {
		elapsedWithUnits = fmt.Sprintf("%ds %dms", seconds, milliseconds)
	} else {
		elapsedWithUnits = fmt.Sprintf("%dms", milliseconds)
	}

	return
}

func ConvertMStoTimestamp(milliseconds int64) (timestamp string) {
	// Convert milliseconds to seconds and nanoseconds
	secs := milliseconds / 1000
	nanos := (milliseconds % 1000) * int64(time.Millisecond)

	// Create a Time object
	t := time.Unix(secs, nanos)

	// Format to ISO 8601 (RFC3339 is a subset of ISO8601)
	timestamp = t.UTC().Format(time.RFC3339)
	return
}
