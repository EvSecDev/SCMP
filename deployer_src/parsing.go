package main

import "fmt"

// Removes header from SSH request payload and returns string text
// Also validates that the payload length matches the payloads header
func StripPayloadHeader(request []byte) (payload string, err error) {
	// Ignore things less than header length
	if len(request) <= 4 {
		err = fmt.Errorf("invalid payload length (did the client send anything?)")
		return
	}

	// Calculate length of payload
	payloadLength := int(request[0])<<24 | int(request[1])<<16 | int(request[2])<<8 | int(request[3])

	// Validate total payload length
	if payloadLength+4 != len(request) {
		err = fmt.Errorf("payload length does not match header metadata")
		return
	}

	// Return payload without header
	payload = string(request[4:])
	return
}
