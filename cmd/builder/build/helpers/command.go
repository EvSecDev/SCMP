// Lower level builder helpers
package helpers

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Runs a go test command streaming and filtering output
func RunTestCommand(cmd exec.Cmd) (err error) {
	var stdout io.ReadCloser
	stdout, err = cmd.StdoutPipe()
	if err != nil {
		err = fmt.Errorf("failed to get stdout pipe for test command: %w", err)
		return
	}

	err = cmd.Start()
	if err != nil {
		err = fmt.Errorf("failed to start test command: %w", err)
		return
	}

	// Stream stdout line by line
	stdoutReader := bufio.NewReader(stdout)
	for {
		line, err := stdoutReader.ReadString('\n')
		if len(line) > 0 {
			if strings.Contains(line, "[no test files]") {
				continue
			}
			if strings.Contains(line, "coverage: 0.0% of statements") {
				continue
			}
			if strings.HasPrefix(line, "PASS") ||
				strings.HasPrefix(line, "goos: ") ||
				strings.HasPrefix(line, "goarch: ") ||
				strings.HasPrefix(line, "cpu: ") ||
				strings.HasPrefix(line, "coverage: ") {
				continue
			}
			fmt.Print(line)
		}
		if err != nil {
			break
		}
	}
	err = cmd.Wait()
	if err != nil {
		err = fmt.Errorf("failed to run test: %w", err)
		return
	}
	return
}
