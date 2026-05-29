package logctx

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// Gets all current logs in logger event buffer. Does not drain events from buffer.
func (logger *Logger) GetFormattedLogLines() (formatted []string) {
	// Copy under lock to avoid holding mutex while sorting/formatting
	logger.mutex.Lock()
	events := make([]Event, len(logger.queue))
	copy(events, logger.queue)
	logger.mutex.Unlock()

	// Stable sort: oldest to newest
	sort.SliceStable(events, func(i, j int) bool {
		ti := events[i].Timestamp
		tj := events[j].Timestamp

		// Zero timestamps sort last
		if ti.IsZero() && tj.IsZero() {
			return false
		}
		if ti.IsZero() {
			return false
		}
		if tj.IsZero() {
			return true
		}
		return ti.Before(tj)
	})

	formatted = make([]string, 0, len(events))
	for _, event := range events {
		msg := event.Format(logger.PrintLevel)

		// Append newlines if not present
		if !strings.HasSuffix(msg, "\n") {
			msg += "\n"
		}

		// Final string
		formatted = append(formatted, msg)
	}
	return
}

// Starts a go routine that reads events and writes formatted output to io.Writer or raw event to event stream channel if present.
// Stops when logger.Done is closed.
// No-op when no logger is present in context.
func StartOutput(ctx context.Context) {
	logger := GetLogger(ctx)
	if logger == nil {
		return
	}
	logger.wg.Add(1)
	go logger.outputWriter()
}

// Background writer to configured outputs
func (logger *Logger) outputWriter() {
	defer logger.wg.Done()

	logger.outMutex.Lock()
	if logger.rawOutput != nil {
		defer close(logger.rawOutput)
	}
	logger.outMutex.Unlock()

	for {
		logger.mutex.Lock()

		// If done and queue is empty, exit
		if len(logger.queue) == 0 {
			select {
			case <-logger.Done:
				logger.mutex.Unlock()
				return
			default:
			}
		}

		// Wait for events
		for len(logger.queue) == 0 {
			select {
			case <-logger.Done:
				logger.mutex.Unlock()
				return
			default:
				logger.cond.Wait()
			}
		}

		logger.outMutex.Lock()
		if logger.rawOutput == nil && logger.formattedOutput == nil {
			// No outputs configured yet, don't drain queue
			logger.outMutex.Unlock()
			return
		}
		logger.outMutex.Unlock()

		// Pop one event from the front of the queue
		event := logger.queue[0]
		logger.queue = logger.queue[1:]
		logger.mutex.Unlock()

		event, printEvent := logger.dedup.handleDuplication(event)
		if !printEvent {
			continue
		}

		logger.outMutex.Lock()

		if logger.rawOutput != nil {
			// Attempt to push to channel, but give up after retries
			for range maxOutputWriteFailures {
				select {
				case logger.rawOutput <- event:
				default:
					time.Sleep(100 * time.Microsecond)
					continue
				}
				break
			}
		}

		if logger.formattedOutput != nil {
			for range maxOutputWriteFailures {
				_, err := fmt.Fprintf(logger.formattedOutput, "%s", event.Format(logger.PrintLevel))
				if err != nil {
					fmt.Fprintf(os.Stderr, "encountered failure writing to log output sink: %s", err.Error())
					time.Sleep(500 * time.Microsecond)
					continue
				}
				break
			}
		}
		logger.outMutex.Unlock()
	}
}
