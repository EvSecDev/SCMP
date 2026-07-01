package input

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"golang.org/x/term"
)

// Prompts user to enter something
func promptUser(userPrompt string) (userResponse string, err error) {
	// Throw error if not in terminal - stdin not available outside terminal for users
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		err = fmt.Errorf("not in a terminal, prompts do not work")
		return
	}

	fmt.Print(userPrompt)
	_, err = fmt.Scanln(&userResponse)
	if err != nil {
		if !strings.HasSuffix(err.Error(), "unexpected newline") {
			err = fmt.Errorf("failed to read input: %w", err)
			return
		}
		err = nil
	}
	userResponse = strings.ToLower(userResponse)
	return
}

// Prompts user for a secret value (does not echo back entered text)
func promptUserForSecret(userPrompt string) (userResponse []byte, err error) {
	fd := int(os.Stdin.Fd())

	// Throw error if not in terminal - stdin not available outside terminal for users
	if !term.IsTerminal(fd) {
		err = fmt.Errorf("not in a terminal, prompts do not work")
		return
	}

	// Save old terminal state
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		err = fmt.Errorf("failed to set terminal raw mode: %w", err)
		return
	}
	defer func() {
		// Restore terminal state upon program exit
		_ = term.Restore(fd, oldState)
		fmt.Println()
	}()

	// Catch signals to ensure cleanup occurs prior to exit
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		_ = term.Restore(fd, oldState)
		fmt.Println()
		os.Exit(1)
	}()

	// Print prompt
	fmt.Print(userPrompt)

	// Read secret input from user
	userResponse, err = term.ReadPassword(fd)
	if err != nil {
		err = fmt.Errorf("error reading password: %w", err)
		return
	}

	return
}
