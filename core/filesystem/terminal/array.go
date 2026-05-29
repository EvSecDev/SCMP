package terminal

import (
	"bufio"
	"fmt"
	"strings"
)

// Dedicated editing menu for arrays
func editStringSlice(reader *bufio.Reader, itemList []string, name string) (modifiedList []string) {
	for {
		var linesPrinted int
		fmt.Printf("==== %s ====\n", name)
		linesPrinted++
		for index, item := range itemList {
			fmt.Printf("%d  %s\n", index+1, item)
			linesPrinted++
		}

		menuOptions := []string{
			fmt.Sprintf("=====%s=====", strings.Repeat("=", len(name))),
			" Add   Edit   Remove  Back",
			"[ a ] [ e# ]  [ r# ]  [ ! ]",
		}
		for _, menuOption := range menuOptions {
			fmt.Println(menuOption)
			linesPrinted++
		}

		fmt.Print("# Type your selection: ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		clearLines(linesPrinted)

		if choice == "" {
			modifiedList = itemList
			return
		}

		switch string(choice[0]) {
		case "a":
			clearLines(1)
			item := promptString(reader, "", "Enter new item")
			itemList = append(itemList, item)
			clearLines(1)
		case "e":
			editIndex, invalid := validateUserIndexChoice(choice, itemList, reader)
			if invalid {
				continue
			}
			clearLines(1)
			newItem := promptString(reader, itemList[editIndex-1], "Enter new value")
			itemList[editIndex-1] = newItem
			clearLines(1)
		case "r":
			removeIndex, invalid := validateUserIndexChoice(choice, itemList, reader)
			if invalid {
				continue
			}
			itemList = append(itemList[:removeIndex-1], itemList[removeIndex:]...)
			clearLines(1)
		case "!":
			modifiedList = itemList
			return
		default:
			fmt.Printf("Unknown option %s\n", choice)
			waitForEnter(reader)
			clearLines(2)
		}
	}
}
