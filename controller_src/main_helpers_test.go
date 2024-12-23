// controller
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

// Unit test for printMessage
func TestPrintMessage(t *testing.T) {
	tests := []struct {
		verbosityLevel      int
		requiredVerbosity   int
		message             string
		expectedOutputMatch bool
		expectedOutput      string
	}{
		{0, 0, "Test message", false, ""},            // No output at verbosity level 0
		{1, 1, "Test message", true, "Test message"}, // Output matches at verbosity level 1
		{2, 1, "Test message", true, "Test message"}, // Output matches at verbosity level 2
		{2, 2, "Test message", true, "Test message"}, // Output matches at verbosity level 2 with timestamp
		{2, 3, "Test message", false, ""},            // No output at verbosity level 2 with higher verbosity required
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("globalVerbosityLevel=%d requiredVerbosity=%d", test.verbosityLevel, test.requiredVerbosity), func(t *testing.T) {
			// Set the global verbosity level for the test
			globalVerbosityLevel = test.verbosityLevel

			old := os.Stdout // keep backup of the real stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Call the printMessage function with the test parameters
			printMessage(test.requiredVerbosity, test.message)

			// Copy the output in a separate goroutine so printing can't block indefinitely
			outputC := make(chan string)
			go func() {
				var buf bytes.Buffer
				io.Copy(&buf, r)
				outputC <- buf.String()
			}()

			// Back to normal state
			w.Close()
			os.Stdout = old // restoring the real stdout
			output := <-outputC

			// Check if the output matches expectations
			if test.expectedOutputMatch {
				// If output is expected, it should contain the test (timestamps are sometimes present, so it wont match exactly)
				if !strings.Contains(output, test.expectedOutput) {
					t.Errorf("expected %q but got %q", test.expectedOutput, output)
				}
			} else {
				// If no output is expected, verify that the buffer is empty
				if output != "" {
					t.Errorf("expected no output but got %q", output)
				}
			}
		})
	}
}
