package terminal

import "fmt"

// Clear lines (no scrolling menu) for low verbosity
func clearLines(maxLines int) {
	for i := 0; i < maxLines; i++ {
		fmt.Print("\033[1A") // move cursor up
		fmt.Print("\033[2K") // clear entire line
	}
}
