package logctx

import (
	"io"
	"sync"
	"time"
)

type CtxKey string

// Log Event Structure
type Event struct {
	Timestamp time.Time // Time when event enters log buffer
	Severity  string
	Tags      []string
	Message   string
}

// Logger Struct
type Logger struct {
	ID         string    // Unique identifier within the program
	CreatedAt  time.Time // Time when logger was created
	PrintLevel int       // Level at which the message should be recorded

	queue []Event // event buffer

	dedup *dedupState // NOT concurrent safe

	// Outputs
	formattedOutput io.Writer
	rawOutput       chan Event
	outMutex        sync.Mutex // Protects switching outputs

	mutex sync.Mutex // protects buffer
	cond  *sync.Cond // condition to signal new events

	Done <-chan struct{} // graceful shutdown (mainly for watcher)
	wg   *sync.WaitGroup // Holds main execution threads until log watchers are done handling events
}

// Tracks successive duplicate messages to prevent log flooding
type dedupState struct {
	lastMsg          string
	repeatCount      int
	lastSuppressTime time.Time
}
