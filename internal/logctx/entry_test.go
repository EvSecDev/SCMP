package logctx

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestLogEvent(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	baseCtx := context.Background()

	ctx := New(
		baseCtx,
		NSTest,
		2, // PrintLevel
		done,
	)

	logger := GetLogger(ctx)
	if logger == nil {
		t.Fatalf("expected logger creation, got nil logger")
	}

	tests := []struct {
		name          string
		logLevel      int
		eventLevel    int
		severity      string
		message       string
		vars          []any
		expectEvents  int
		expectMessage string
	}{
		{
			name:          "event level <= print level is logged",
			logLevel:      2,
			eventLevel:    1,
			severity:      InfoLog,
			message:       "hello world",
			expectEvents:  1,
			expectMessage: "hello world",
		},
		{
			name:         "event level > print level is dropped",
			logLevel:     1,
			eventLevel:   3,
			severity:     InfoLog,
			message:      "should not appear",
			expectEvents: 0,
		},
		{
			name:          "error severity bypasses level filtering",
			logLevel:      0,
			eventLevel:    5,
			severity:      ErrorLog,
			message:       "fatal error",
			expectEvents:  1,
			expectMessage: "fatal error",
		},
		{
			name:          "formatted message with vars",
			logLevel:      3,
			eventLevel:    2,
			severity:      InfoLog,
			message:       "value=%d",
			vars:          []any{42},
			expectEvents:  1,
			expectMessage: "value=42",
		},
		{
			name:          "no formatting when no format verbs",
			logLevel:      3,
			eventLevel:    2,
			severity:      InfoLog,
			message:       "log this message",
			vars:          []any{123},
			expectEvents:  1,
			expectMessage: "log this message",
		},
		{
			name:          "format verb but no variables for format",
			logLevel:      3,
			eventLevel:    2,
			severity:      InfoLog,
			message:       "log this message %d",
			vars:          []any{},
			expectEvents:  1,
			expectMessage: "log this message %d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset queue
			logger.mutex.Lock()
			logger.queue = []Event{}
			logger.mutex.Unlock()

			SetLogLevel(ctx, tt.logLevel)

			if tt.vars != nil {
				LogEvent(ctx, tt.eventLevel, tt.severity, tt.message, tt.vars...)
			} else {
				LogEvent(ctx, tt.eventLevel, tt.severity, tt.message)
			}

			logger.mutex.Lock()
			defer logger.mutex.Unlock()

			if got := len(logger.queue); got != tt.expectEvents {
				t.Fatalf("expected %d events, got %d", tt.expectEvents, got)
			}

			if tt.expectEvents == 1 {
				ev := logger.queue[0]

				if ev.Severity != tt.severity {
					t.Fatalf("severity mismatch: got %q want %q", ev.Severity, tt.severity)
				}

				if ev.Message != tt.expectMessage {
					t.Fatalf("message mismatch: got %q want %q", ev.Message, tt.expectMessage)
				}

				if ev.Timestamp.IsZero() {
					t.Fatal("event timestamp is zero")
				}

				if time.Since(ev.Timestamp) > time.Second {
					t.Fatalf("event timestamp too old: %v", ev.Timestamp)
				}
			}
		})
	}
}

func TestGetFormattedLogLines_ChronologicalBatching(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	ctx := New(
		context.Background(),
		NSTest,
		5,
		done,
	)

	l := GetLogger(ctx)
	if l == nil {
		t.Fatal("logger not found in context")
		return // Satisfy staticcheck SA5011
	}

	base := time.Now()

	// Construct deliberately out-of-order events
	e1 := Event{
		Timestamp: base.Add(3 * time.Second),
		Severity:  InfoLog,
		Message:   "third",
	}
	e2 := Event{
		Timestamp: base.Add(1 * time.Second),
		Severity:  InfoLog,
		Message:   "first",
	}
	e3 := Event{
		Timestamp: base.Add(2 * time.Second),
		Severity:  InfoLog,
		Message:   "second",
	}
	e4 := Event{
		Timestamp: time.Time{}, // zero timestamp
		Severity:  InfoLog,
		Message:   "zero",
	}

	// Insert in non-chronological order (simulating multiple producers)
	l.mutex.Lock()
	l.queue = []Event{e1, e4, e2, e3}
	l.mutex.Unlock()

	lines := l.GetFormattedLogLines()

	if len(lines) != 4 {
		t.Fatalf("expected 4 log lines, got %d", len(lines))
	}

	// Expected order after sorting:
	// first (t+1s)
	// second (t+2s)
	// third (t+3s)
	// zero timestamp last
	expectedOrder := []string{
		"first",
		"second",
		"third",
		"zero",
	}

	for i, want := range expectedOrder {
		if !strings.Contains(lines[i], want) {
			t.Fatalf(
				"line %d ordering mismatch: got %q, want message containing %q",
				i,
				lines[i],
				want,
			)
		}

		if !strings.HasSuffix(lines[i], "\n") {
			t.Fatalf("line %d missing trailing newline: %q", i, lines[i])
		}
	}
}
