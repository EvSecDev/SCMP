// Test utility functions meant to be used across many test functions in other packages
package utils

import (
	"context"
	"errors"
	"fmt"
	"scmp/internal/logctx"
	"strings"
)

// Takes a filter and looks for warnings/errors in the logctx log buffer that match (contains matching).
// Exclude filters are also contains match.
// Any error/warn not matching will reside in err.
func MatchLogCtxErrors(ctx context.Context, searchFilter string, excludeFilters []string) (matches bool, err error) {
	logger := logctx.GetLogger(ctx)
	lines := logger.GetFormattedLogLines()

	var foundMatch bool
	var extraErrors []string

	// Search event buffer
	for _, line := range lines {
		if strings.Contains(line, "["+logctx.InfoLog+"]") {
			continue
		}
		if len(excludeFilters) > 0 {
			var excludeLine bool
			for _, excludeFilter := range excludeFilters {
				if strings.Contains(line, excludeFilter) {
					excludeLine = true
					break
				}
			}
			if excludeLine {
				// Do not add exclude to found errors (for either extra or matched)
				continue
			}
		}
		if searchFilter != "" && strings.Contains(line, searchFilter) {
			foundMatch = true
		} else {
			extraErrors = append(extraErrors, line)
		}
	}

	// Create specific errors for each possible condition
	if searchFilter != "" {
		if !foundMatch && len(extraErrors) == 0 {
			err = fmt.Errorf("expected error '%s', but got none", searchFilter)
			return
		} else if !foundMatch && len(extraErrors) > 0 {
			err = fmt.Errorf("expected error '%s', but only found:\n%v", searchFilter, extraErrors)
			return
		} else if foundMatch && len(extraErrors) == 0 {
			matches = true
			return
		} else if foundMatch && len(extraErrors) > 0 {
			err = fmt.Errorf("expected only error '%s', but also found:\n%v", searchFilter, extraErrors)
			return
		}
	} else {
		if len(extraErrors) > 0 {
			err = fmt.Errorf("expected no error, but found:\n%v", extraErrors)
		}
	}
	return
}

// Checks if provided error matches expected, mismatches reported in err.
// Matches is true when gotError contains expectedError
func MatchErrorString(gotError error, expectedError string) (matches bool, err error) {
	if gotError != nil {
		if expectedError == "" {
			err = fmt.Errorf("expected no error, but got '%v'", gotError)
		} else if strings.Contains(gotError.Error(), expectedError) {
			matches = true
		} else {
			err = fmt.Errorf("expected error '%s', but got error '%v'", expectedError, gotError)
		}
	} else {
		if expectedError != "" {
			err = fmt.Errorf("expected error '%s', but got none", expectedError)
		}
	}
	return
}

// Checks if provided error contains the wrapped expected error somewhere in the tree.
// Matches is true when  present. Mismatches reported in err
func MatchWrappedError(gotError error, expectedError error) (matches bool, err error) {
	if gotError != nil {
		if expectedError == nil {
			err = fmt.Errorf("expected no error, but got '%v'", gotError)
		} else if errors.Is(gotError, expectedError) {
			matches = true
		} else {
			err = fmt.Errorf("expected error '%v', but got error '%v'", expectedError, gotError)
		}
	} else {
		if expectedError != nil {
			err = fmt.Errorf("expected error '%v', but got none", expectedError)
		}
	}
	return
}
