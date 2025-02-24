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

// Unit Test
func TestFilterHostGroups(t *testing.T) {
	// Mock global
	config.UniversalDirectory = "UniversalConfs"

	tests := []struct {
		endpointName                 string
		universalGroupsCSV           string
		ignoreUniversalString        string
		expectedHostIgnoresUniversal bool
		expectedHostUniversalGroups  map[string]struct{}
		expectedAllUniversalGroups   map[string][]string
	}{
		{
			endpointName:                 "host1",
			universalGroupsCSV:           "group1,group2",
			ignoreUniversalString:        "no",
			expectedHostIgnoresUniversal: false,
			expectedHostUniversalGroups: map[string]struct{}{
				"group1":         {},
				"group2":         {},
				"UniversalConfs": {}, // Default universal group should be added
			},
			expectedAllUniversalGroups: map[string][]string{
				"group1":         {"host1"},
				"group2":         {"host1"},
				"UniversalConfs": {"host1"},
			},
		},
		{
			endpointName:                 "host2",
			universalGroupsCSV:           "group1",
			ignoreUniversalString:        "yes",
			expectedHostIgnoresUniversal: true,
			expectedHostUniversalGroups: map[string]struct{}{
				"group1": {},
			},
			expectedAllUniversalGroups: map[string][]string{
				"group1": {"host2"},
			},
		},
		{
			endpointName:                 "host3",
			universalGroupsCSV:           "",
			ignoreUniversalString:        "no",
			expectedHostIgnoresUniversal: false,
			expectedHostUniversalGroups: map[string]struct{}{
				"UniversalConfs": {},
			},
			expectedAllUniversalGroups: map[string][]string{
				"UniversalConfs": {"host3"},
			},
		},
	}

	for _, test := range tests {
		// Reset global state for each test
		config.AllUniversalGroups = make(map[string][]string)

		t.Run(test.endpointName, func(t *testing.T) {
			// Run the function
			HostIgnoresUniversal, HostUniversalGroups := filterHostGroups(test.endpointName, test.universalGroupsCSV, test.ignoreUniversalString)

			// Check if the results match expectations
			if HostIgnoresUniversal != test.expectedHostIgnoresUniversal {
				t.Errorf("expected HostIgnoresUniversal to be %v, got %v", test.expectedHostIgnoresUniversal, HostIgnoresUniversal)
			}

			if len(HostUniversalGroups) != len(test.expectedHostUniversalGroups) {
				t.Errorf("expected HostUniversalGroups length to be %d, got %d", len(test.expectedHostUniversalGroups), len(HostUniversalGroups))
			} else {
				for group := range test.expectedHostUniversalGroups {
					if _, exists := HostUniversalGroups[group]; !exists {
						t.Errorf("expected HostUniversalGroups to contain group %s", group)
					}
				}
			}

			// Check the global map AllUniversalGroups
			for group, expectedHosts := range test.expectedAllUniversalGroups {
				if len(config.AllUniversalGroups[group]) != len(expectedHosts) {
					t.Errorf("expected %d hosts in group %s, got %d", len(expectedHosts), group, len(config.AllUniversalGroups[group]))
				} else {
					for _, expectedHost := range expectedHosts {
						found := false
						for _, host := range config.AllUniversalGroups[group] {
							if host == expectedHost {
								found = true
								break
							}
						}
						if !found {
							t.Errorf("expected host %s in group %s, but it was not found", expectedHost, group)
						}
					}
				}
			}
		})
	}
}
