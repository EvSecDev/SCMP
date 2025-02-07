// controller
package main

import "testing"

func TestParseEndpointAddress(t *testing.T) {
	tests := []struct {
		endpointIP   string
		port         string
		expectedAddr string
		expectError  bool
	}{
		// Valid IPv4 test case
		{
			endpointIP:   "192.168.1.1",
			port:         "8080",
			expectedAddr: "192.168.1.1:8080",
			expectError:  false,
		},
		// Valid IPv6 test case
		{
			endpointIP:   "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
			port:         "8080",
			expectedAddr: "[2001:0db8:85a3:0000:0000:8a2e:0370:7334]:8080",
			expectError:  false,
		},
		// Invalid IP address
		{
			endpointIP:   "999.999.999.999",
			port:         "8080",
			expectedAddr: "",
			expectError:  true,
		},
		// Invalid port, out of range (below 1)
		{
			endpointIP:   "192.168.1.1",
			port:         "0",
			expectedAddr: "",
			expectError:  true,
		},
		// Invalid port, out of range (above 65535)
		{
			endpointIP:   "192.168.1.1",
			port:         "70000",
			expectedAddr: "",
			expectError:  true,
		},
		// Invalid IPv4 address format
		{
			endpointIP:   "192.168.1",
			port:         "8080",
			expectedAddr: "",
			expectError:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.endpointIP+"_"+test.port, func(t *testing.T) {
			result, err := ParseEndpointAddress(test.endpointIP, test.port)

			if test.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
				if result != test.expectedAddr {
					t.Errorf("expected address '%s' but got '%s'", test.expectedAddr, result)
				}
			}
		})
	}
}
