package logctx

import (
	"context"
	"sync"
	"time"
)

// Logger Constructor.
// Embeds logger in returned context using provided context as base.
func New(baseCtx context.Context, id string, logLevel int, done <-chan struct{}) (ctxLogger context.Context) {
	// loglevel
	//
	// Integer for printing increasingly detailed information as program progresses
	//
	//	0 - None: quiet (prints nothing but errors)
	//	1 - Standard: normal progress messages
	//	2 - Progress: more progress messages (no actual data outputted)
	//	3 - Data: shows limited data being processed
	//	4 - FullData: shows full data being processed
	//	5 - Debug: shows extra data during processing (raw bytes)

	logger := &Logger{
		ID:         id,
		CreatedAt:  time.Now(),
		queue:      make([]Event, 0),
		dedup:      &dedupState{},
		Done:       done,
		PrintLevel: logLevel,
		wg:         &sync.WaitGroup{},
	}
	logger.cond = sync.NewCond(&logger.mutex)

	ctxLogger = context.WithValue(baseCtx, LoggerKey, logger)
	return
}

// Change the logger's level
func SetLogLevel(ctx context.Context, newLevel int) {
	logger := GetLogger(ctx)
	if logger != nil {
		logger.mutex.Lock()
		defer logger.mutex.Unlock()
		logger.PrintLevel = newLevel
	}
}

// Extracts Logger from context or returns nil
func GetLogger(ctx context.Context) (logger *Logger) {
	logger, ok := ctx.Value(LoggerKey).(*Logger)
	if ok {
		return
	}
	logger = nil
	return
}

// Creates new context with isolated tags from the baseCtx while adding the addTags list.
func NewCancelWithValues(baseCtx context.Context, addTags ...string) (ctx context.Context, cancel context.CancelFunc) {
	for _, newTag := range addTags {
		baseCtx = AppendCtxTag(baseCtx, newTag)
	}
	ctx, cancel = context.WithCancel(baseCtx)
	return
}
