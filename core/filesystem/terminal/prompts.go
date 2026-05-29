// Package for filesystem terminal user interface used for safely interacting with custom file structures
package terminal

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Prompt user for new string value
// "-"  input returns empty string
// Empty input returns currentValue
func promptString(reader *bufio.Reader, currentValue string, prompt string) (userInput string) {
	if currentValue != "" {
		fmt.Print("[Current: " + currentValue + "] " + prompt + ": ")
	} else {
		fmt.Print(prompt + ": ")
	}
	text, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read input: %v\n", err)
		return
	}
	userInput = strings.TrimSpace(text)

	switch userInput {
	case "":
		// Entering nothing uses existing value
		userInput = currentValue
	case "-":
		// Special character to actually delete field value
		userInput = ""
	}
	return
}

// Prompt user for new number
// "-"  input returns empty string
// Empty input returns currentValue
func promptInt(reader *bufio.Reader, currentValue int, prompt string) (userInput int) {
	currentValueText := strconv.Itoa(currentValue)
	for {
		if currentValue != 0 {
			fmt.Print("[Current: " + currentValueText + "] " + prompt + ": ")
		} else {
			fmt.Print(prompt + ": ")
		}
		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(text)

		switch text {
		case "":
			// Entering nothing uses existing value
			text = currentValueText
		case "-":
			// Special character to actually delete field value
			text = "0"
		}

		num, err := strconv.Atoi(text)
		if err == nil {
			userInput = num
			return
		}
		fmt.Println("Invalid number, try again.")
	}
}

// Blocks until newline is encountered (from user)
func waitForEnter(reader *bufio.Reader) {
	fmt.Print("Press Enter to continue...")
	_, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read input: %v\n", err)
	}
}
