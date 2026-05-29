package logctx

import (
	"fmt"
	"strings"
	"time"
)

// Stringify full event
func (event Event) Format(printLevel int) (text string) {
	// Override: Std Info prints raw
	rawPrintMode := printLevel == VerbosityStandard && event.Severity == InfoLog

	// Only print parts that are present
	var parts []string
	if !rawPrintMode && !event.Timestamp.IsZero() {
		parts = append(parts, fmt.Sprintf("[%s]", padTimestamp(event.Timestamp)))
	}

	if !rawPrintMode && len(event.Tags) > 0 {
		tagPrefixes := "["
		tagPrefixes += strings.Join(event.Tags, "/")
		tagPrefixes += "]"
		parts = append(parts, tagPrefixes)
	}

	if event.Severity != "" {
		if printLevel > VerbosityStandard {
			// Normal verbose print
			parts = append(parts, fmt.Sprintf("[%s]", event.Severity))
		} else if event.Severity != InfoLog {
			// Simple prefix for standard verbosity and non-info log
			parts = append(parts, fmt.Sprintf("%s:", event.Severity))
		}
	}

	if event.Message != "" {
		parts = append(parts, event.Message)
	}

	text = strings.Join(parts, " ")
	// No newline, message creator determines newlines
	return
}

// Ensures fixed length strings for timestamps
func padTimestamp(timestamp time.Time) (formatted string) {
	formatted = timestamp.Format(time.RFC3339Nano)

	lastColon := strings.LastIndex(formatted, ":")
	if lastColon == -1 {
		// malformed, return as-is
		return
	}

	majorFields := strings.Split(formatted, ".")
	if len(majorFields) > 2 || len(majorFields) < 1 {
		// malformed
		return
	}
	if len(majorFields) == 1 {
		// No nanoseconds (at 0) - add padding
		formattedPrefix := formatted[:lastColon+3]
		formattedSuffix := formatted[lastColon+3:]
		newFormatted := formattedPrefix + ".000000000" + formattedSuffix

		// Re-split
		majorFields = strings.Split(newFormatted, ".")
		if len(majorFields) != 2 {
			// failed parsing, return as is
			return
		}
		formatted = newFormatted
	}

	minorFields := strings.Split(majorFields[1], "-")
	if len(minorFields) != 2 {
		return
	}

	tsPrefix := majorFields[0]
	nanoseconds := minorFields[0]
	timezoneOffset := minorFields[1]

	// Pad the nanoseconds part to ensure it's 9 digits long
	for len(nanoseconds) < 9 {
		nanoseconds = "0" + nanoseconds
	}

	// Rebuild the timestamp with padded nanoseconds and the original timezone offset
	formatted = fmt.Sprintf("%s.%s-%s", tsPrefix, nanoseconds, timezoneOffset)
	return
}
