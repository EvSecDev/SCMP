package build

import (
	"fmt"
	"os"
	"strings"
)

func printErr(indent int, message string, vars ...any) {
	_, _ = fmt.Fprintf(os.Stdout, "%s%s[-] Error:%s %s\n",
		strings.Repeat(" ", indent), colorRed, noColor, formatMessage(message, vars...))
}

func printWarn(indent int, message string, vars ...any) {
	_, _ = fmt.Fprintf(os.Stdout, "%s%s[?] WARNING:%s %s\n",
		strings.Repeat(" ", indent), colorYellow, noColor, formatMessage(message, vars...))
}

func printSuccess(indent int, message string, vars ...any) {
	if strings.ToLower(message) == "done" {
		message = colorGreen + colorBold + "DONE" + noColor
	}
	_, _ = fmt.Fprintf(os.Stdout, "%s%s[+]%s %s\n",
		strings.Repeat(" ", indent), colorGreen, noColor, formatMessage(message, vars...))
}

func printInfo(indent int, message string, vars ...any) {
	_, _ = fmt.Fprintf(os.Stdout, "%s%s[*]%s %s\n",
		strings.Repeat(" ", indent), colorBold, noColor, formatMessage(message, vars...))
}

func formatMessage(message string, vars ...any) (newMsg string) {
	// vars might be empty - check to omit formatting
	if len(vars) == 0 || (!strings.Contains(message, "%") && !strings.Contains(message, `%%`)) {
		// Avoiding 'extra' print to log entries
		newMsg = message
	} else {
		// Maintain %w error wrapping compatibility
		message = strings.ReplaceAll(message, "%w", "%v")

		newMsg = fmt.Sprintf(message, vars...)
	}
	return
}
