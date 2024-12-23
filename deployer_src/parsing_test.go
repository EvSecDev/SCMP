package main

import "testing"

func TestStripPayloadHeader(t *testing.T) {
	tests := []struct {
		name            string
		request         []byte
		expectedPayload string
		expectedError   string
	}{
		{"valid payload", []byte{0, 0, 0, 5, 'H', 'e', 'l', 'l', 'o'}, "Hello", ""},
		{"invalid payload length (too short)", []byte{0, 0, 0, 5}, "", "invalid payload length (did the client send anything?)"},
		{"mismatched payload length", []byte{0, 0, 0, 5, 'H', 'e', 'l'}, "", "payload length does not match header metadata"},
		{"empty request", []byte{}, "", "invalid payload length (did the client send anything?)"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload, err := StripPayloadHeader(test.request)
			if err != nil && err.Error() != test.expectedError {
				t.Errorf("expected error '%s', got: '%s'", test.expectedError, err)
			}
			if payload != test.expectedPayload {
				t.Errorf("expected payload '%s', got: '%s'", test.expectedPayload, payload)
			}
		})
	}
}
