package logctx

import (
	"fmt"
	"time"
)

// Logs event
func (logger *Logger) log(eventLevel int, eventSeverity string, tags []string, fullMessage string) {
	logger.mutex.Lock()
	currentLevel := logger.PrintLevel
	logger.mutex.Unlock()

	if eventLevel > currentLevel && eventSeverity != ErrorLog {
		return
	}

	event := Event{
		Timestamp: time.Now(),
		Tags:      tags,
		Severity:  eventSeverity,
		Message:   fullMessage,
	}

	logger.mutex.Lock()
	logger.queue = append(logger.queue, event)
	logger.cond.Signal() // Notify watcher that new event is available
	logger.mutex.Unlock()
}

// Deduplication logic
// Duplicate events older than the deduplication window are not considered duplicates.
// Purely meant for highly repetitive message suppression to prevent excessive noise.
func (state *dedupState) handleDuplication(latestEvent Event) (newEvent Event, printEvent bool) {
	now := time.Now()

	if latestEvent.Message != "" &&
		now.Sub(latestEvent.Timestamp) <= dedupWindow &&
		latestEvent.Message == state.lastMsg {

		state.repeatCount++
		if state.repeatCount >= minRepeats && now.Sub(state.lastSuppressTime) >= suppressCooldown {
			// Suppression message once per minute max
			newEvent = Event{
				Timestamp: latestEvent.Timestamp,
				Tags:      latestEvent.Tags,
				Severity:  latestEvent.Severity,
				Message:   fmt.Sprintf("Suppressed %d repeated messages: %s", state.repeatCount, state.lastMsg),
			}
			state.lastSuppressTime = now
			state.repeatCount = 0

			// Print suppression message
			printEvent = true
		} else {
			// Within duplication and window, skip print
			printEvent = false
		}
	} else {
		// Reset counter if message changes or window exceeded
		state.lastMsg = latestEvent.Message
		state.repeatCount = 1
		newEvent = latestEvent
		printEvent = true
	}

	return
}
