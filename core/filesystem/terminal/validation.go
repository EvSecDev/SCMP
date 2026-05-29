package terminal

import (
	"bufio"
	"fmt"
	"strconv"
)

// Validates user chosen index for given array
func validateUserIndexChoice(userChoice string, itemList []string, reader *bufio.Reader) (index int, invalid bool) {
	if len(itemList) == 0 {
		fmt.Println("List is empty")
		waitForEnter(reader)
		clearLines(3)
		invalid = true
		return
	}
	if len(userChoice) < 2 {
		fmt.Println("Must include number following 'r'")
		waitForEnter(reader)
		clearLines(3)
		invalid = true
		return
	}
	chosenIndex, err := strconv.Atoi(string(userChoice[1:]))
	if err != nil {
		fmt.Printf("Invalid numeric choice: %v\n", err)
		waitForEnter(reader)
		clearLines(3)
		invalid = true
		return
	}
	if chosenIndex < 1 || chosenIndex > len(itemList) {
		fmt.Println("Choice out of range for list length")
		waitForEnter(reader)
		clearLines(3)
		invalid = true
		return
	}
	index = chosenIndex
	return
}
