package logctx

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestWatcher_WaitWakeAndDedup(t *testing.T) {
	done := make(chan struct{})

	ctx := New(
		context.Background(),
		NSTest,
		5,
		done,
	)

	logger := GetLogger(ctx)
	if logger == nil {
		t.Fatal("logger not found in context")
	}

	var output bytes.Buffer

	logger.SetFormattedOutput(&output)

	// Start watcher
	StartOutput(ctx)

	// Ensure watcher is waiting (queue empty)
	// Nothing logged yet; watcher should be blocked on cond.Wait()

	// Explicit wake should not crash or write anything
	logger.Wake()

	if output.Len() != 0 {
		t.Fatalf("unexpected output before events: %q", output.String())
	}

	// Log repeated messages to trigger dedup
	const repeats = 11
	msg := "duplicate-message"

	for i := 0; i < repeats; i++ {
		LogEvent(ctx, 1, InfoLog, msg)
	}

	// Wake watcher in case it is waiting
	logger.Wake()

	// Shut down watcher cleanly
	close(done)
	logger.Wake() // ensure it exits wait

	logger.Wait() // must not block

	out := output.String()
	if out == "" {
		t.Fatal("expected output, got empty string")
	}

	// The original message should appear at least once
	if !strings.Contains(out, msg) {
		t.Fatalf("expected original message in output, got:\n%s", out)
	}

	// Suppression message should appear
	if !strings.Contains(out, "Suppressed") {
		t.Fatalf("expected suppression message, got:\n%s", out)
	}

	// Suppression count should be present
	if !strings.Contains(out, "repeated messages") {
		t.Fatalf("expected repeated message count, got:\n%s", out)
	}
}

func TestWatcher_StartBeforeOutputSet(t *testing.T) {
	done := make(chan struct{})
	ctx := New(context.Background(), NSTest, 5, done)

	logger := GetLogger(ctx)
	if logger == nil {
		t.Fatal("logger not found")
	}

	// Start writer BEFORE setting output
	StartOutput(ctx)

	// Small delay to increase chance writer runs first
	time.Sleep(10 * time.Millisecond)

	var buf bytes.Buffer
	logger.SetFormattedOutput(&buf)

	LogEvent(ctx, 1, InfoLog, "hello")

	logger.Wake()

	close(done)
	logger.Wake()
	logger.Wait()

	if !strings.Contains(buf.String(), "hello") {
		t.Fatalf("expected log output, got: %q", buf.String())
	}
}

func TestWatcher_ShutdownFlushesQueue(t *testing.T) {
	done := make(chan struct{})
	ctx := New(context.Background(), NSTest, 5, done)

	logger := GetLogger(ctx)
	var buf bytes.Buffer
	logger.SetFormattedOutput(&buf)

	StartOutput(ctx)

	LogEvent(ctx, 1, InfoLog, "before shutdown")

	// Immediately shutdown
	close(done)
	logger.Wake()
	logger.Wait()

	out := buf.String()
	if !strings.Contains(out, "before shutdown") {
		t.Fatalf("expected message to be flushed, got: %q", out)
	}
}

func TestWatcher_RawAndFormattedOutput(t *testing.T) {
	done := make(chan struct{})
	ctx := New(context.Background(), NSTest, 5, done)

	logger := GetLogger(ctx)

	var buf bytes.Buffer
	logger.SetFormattedOutput(&buf)
	raw := logger.SetRawOutput()

	StartOutput(ctx)

	LogEvent(ctx, 1, InfoLog, "dual")

	close(done)
	logger.Wake()
	logger.Wait()

	select {
	case ev := <-raw:
		if ev.Message != "dual" {
			t.Fatalf("unexpected raw message: %+v", ev)
		}
	default:
		t.Fatal("expected raw event")
	}

	if !strings.Contains(buf.String(), "dual") {
		t.Fatalf("expected formatted output, got: %q", buf.String())
	}
}
