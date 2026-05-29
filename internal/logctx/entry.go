// Central logging system. Buffers messages and writes to configured outputs
package logctx

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// Wrapper - log event as Standard verbosity and Info severity
func LogStdInfo(ctx context.Context, message string, vars ...any) {
	LogEvent(ctx, VerbosityStandard, InfoLog, message, vars...)
}

// Wrapper - log event as Standard verbosity and Warning severity
func LogStdWarn(ctx context.Context, message string, vars ...any) {
	LogEvent(ctx, VerbosityStandard, WarnLog, message, vars...)
}

// Wrapper - log event as Standard verbosity and Error severity
func LogStdErr(ctx context.Context, message string, vars ...any) {
	LogEvent(ctx, VerbosityStandard, ErrorLog, message, vars...)
}

// Wrapper - log event as Standard verbosity and Fatal severity
func LogStdFatal(ctx context.Context, message string, vars ...any) {
	LogEvent(ctx, VerbosityStandard, FatalLog, message, vars...)
}

// Entry for logging events.
// If event level is above the current set logger level, message will not be recorded.
// If severity is an error, event level is not considered and message is recorded.
// Log buffer is backed by deduplication volume to ensure consecutive identical messages do not flood logs.
func LogEvent(ctx context.Context, eventLevel int, severity string, message string, vars ...any) {
	// Retrieve current tag list
	tags := GetTagList(ctx)

	// Get logger pointer
	logger := GetLogger(ctx)
	if logger != nil {
		var newMsg string

		// vars might be empty - check to omit formatting
		if len(vars) == 0 || (!strings.Contains(message, "%") && !strings.Contains(message, `%%`)) {
			// Avoiding 'extra' print to log entries
			newMsg = message
		} else {
			// Maintain %w error wrapping compatibility
			message = strings.ReplaceAll(message, "%w", "%v")

			newMsg = fmt.Sprintf(message, vars...)
		}
		logger.log(eventLevel, severity, tags, newMsg)
	}
}

// Set an output on logger that will receive formatted text logs from the watcher
func (logger *Logger) SetFormattedOutput(sink io.Writer) {
	logger.outMutex.Lock()
	logger.formattedOutput = sink
	logger.outMutex.Unlock()
}

// Removes formatted output from logger
func (logger *Logger) UnsetFormattedOutput() {
	logger.outMutex.Lock()
	logger.formattedOutput = nil
	logger.outMutex.Unlock()
}

// Set output on logger that provides a channel containing streaming raw Events
func (logger *Logger) SetRawOutput() (eventStream <-chan Event) {
	logger.outMutex.Lock()
	logger.rawOutput = make(chan Event, 64)
	eventStream = logger.rawOutput // Only returning receive only channel
	logger.outMutex.Unlock()
	return
}

// Removes raw output on logger (raw Event channel)
func (logger *Logger) UnsetRawOutput() {
	logger.outMutex.Lock()
	logger.rawOutput = nil
	logger.outMutex.Unlock()
}

// Hold main thread exit until logger is finished its work
func (logger *Logger) Wait() {
	logger.wg.Wait()
}

// Wake signals/broadcasts to any goroutines waiting on the condition variable
func (logger *Logger) Wake() {
	logger.mutex.Lock()
	defer logger.mutex.Unlock()
	logger.cond.Broadcast()
}
