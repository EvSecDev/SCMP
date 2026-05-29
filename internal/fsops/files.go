// Package for generic filesystem operations
package fsops

import "os"

// Checks whether given path exists or not (will be false if error)
func FileExists(path string) (exists bool) {
	_, err := os.Stat(path)
	if err == nil {
		exists = true
		return
	}
	if os.IsNotExist(err) {
		exists = false
		return
	}
	exists = false
	return
}
