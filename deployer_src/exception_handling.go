package main

import (
	"fmt"
	"os"
)

// ###################################
//      EXCEPTION HANDLING
// ###################################

func logError(errorDescription string, errorMessage error, FatalError bool) {
	// return early if no error to process
	if errorMessage == nil {
		return
	}
	// Log and exit if requested
	if FatalError {
		fmt.Printf("%s: %v\n", errorDescription, errorMessage)
		os.Exit(1)
	}
	// Just print the error otherwise
	fmt.Printf("%s: %v\n", errorDescription, errorMessage)
}
