package global

import (
	"context"
	"fmt"
	"runtime"
)

// Retrieves a value from context by key and asserts its type.
// Panics if the key is missing or the type assertion fails.
// varDescName: a descriptive name for the value from the caller (for easier tracing)
// key: the context key for stored value
// expectedType: a string describing the expected type
func AssertFromContext[T any](ctx context.Context, varDescName string, key any, expectedType string) (val T) {
	raw := ctx.Value(key)
	if raw == nil {
		panicWithCallerInfo(ctx, varDescName, key, expectedType, "value missing")
	}

	casted, ok := raw.(T)
	if !ok {
		panicWithCallerInfo(ctx, varDescName, key, expectedType, fmt.Sprintf("type assertion failed, got %T", raw))
	}

	return casted
}

func panicWithCallerInfo(ctx context.Context, varDescName string, key any, expectedType string, reason string) {
	pc, file, line, ok := runtime.Caller(2) // depth 2, where AssertFromContext was called
	if !ok {
		file = "unknown"
		line = 0
	}
	fn := runtime.FuncForPC(pc).Name()

	userCtx := ctx.Value(UserKey)
	panic(fmt.Sprintf(
		"User Context: '%v': %s for variable '%v' (key: %v, expected type: %s) at %s:%d in function %s",
		userCtx, reason, varDescName, key, expectedType, file, line, fn,
	))
}

// Asserts that raw is of type T, panics with detailed info if not.
func AssertType[T any](raw any, varDescName string, expectedType string) T {
	casted, ok := raw.(T)
	if !ok {
		pc, file, line, ok := runtime.Caller(2) // depth 2: caller of AssertType
		if !ok {
			file = "unknown"
			line = 0
		}
		fn := runtime.FuncForPC(pc).Name()

		panic(fmt.Sprintf(
			"Type assertion failed for variable '%v' (expected type: %s, got type: %T) at %s:%d in function %s",
			varDescName, expectedType, raw, file, line, fn,
		))
	}
	return casted
}
