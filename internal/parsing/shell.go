package parsing

import "bytes"

// Handles re-adding actual redirection characters (<<<, >, >>) in raw JSON bytes of metadata header
func UnescapeShellRedirectors(rawJSON []byte) (correctedJSON []byte) {
	// Restore <<< (escaped as \u003c\u003c\u003c)
	rawJSON = bytes.ReplaceAll(rawJSON, []byte(`\u003c\u003c\u003c`), []byte("<<<"))

	// Restore >> (escaped as \u003e\u003e)
	rawJSON = bytes.ReplaceAll(rawJSON, []byte(`\u003e\u003e`), []byte(">>"))

	// Restore > (escaped as \u003e)
	rawJSON = bytes.ReplaceAll(rawJSON, []byte(`\u003e`), []byte(">"))

	correctedJSON = rawJSON
	return
}
