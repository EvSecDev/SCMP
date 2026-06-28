package cli

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

const (
	helpMenuTrailer = `
Report bugs to: dev@evsec.net
SCMP home page: <https://github.com/EvSecDev/SCMP>
General help using GNU software: <https://www.gnu.org/gethelp/>
`
)

// Full standardized help menu (wraps option printer as well).
// Commands is the lineage, last is current command
func PrintHelpMenu(fs *flag.FlagSet, commands []string, rootCmd *CommandSet) {
	const baseIndentSpaces = 2

	// Strict lineage resolution
	curCmdSet, parentStack, err := ResolveCommand(commands, rootCmd)
	if err != nil {
		fmt.Printf("Unknown command: %s\n", strings.Join(commands, " "))
		return
	}

	// Build full usage path
	usageParts := []string{os.Args[0]}
	// Append parent commands
	for _, p := range parentStack {
		usageParts = append(usageParts, p.CommandName)
	}
	usageParts = append(usageParts, curCmdSet.CommandName)

	// Don't actually include the root name
	if len(usageParts) > 1 && usageParts[1] == RootCLICommand {
		usageParts = append(usageParts[:1], usageParts[2:]...)
	}

	// Add child commands or usage options
	if len(curCmdSet.ChildCommands) > 1 {
		usageParts = append(usageParts, "[subcommand]")
	} else if len(curCmdSet.ChildCommands) == 1 {
		for name := range curCmdSet.ChildCommands {
			usageParts = append(usageParts, name)
		}
	}
	if curCmdSet.UsageOption != "" {
		usageParts = append(usageParts, curCmdSet.UsageOption)
	}

	fmt.Printf("Usage: %s\n\n", strings.Join(usageParts, " "))

	// Description
	if curCmdSet == rootCmd {
		fmt.Println(curCmdSet.Description)
		fmt.Println(curCmdSet.FullDescription)
		fmt.Println()
	} else if curCmdSet.FullDescription != "" {
		fmt.Println("  Description:")
		fmt.Printf("    %s\n\n", curCmdSet.FullDescription)
	}

	// Subcommands
	if len(curCmdSet.ChildCommands) > 0 {
		indent := strings.Repeat(" ", baseIndentSpaces)
		fmt.Printf("%sSubcommands:\n", indent)

		// Compute max length for padding
		maxLen := 0
		for name := range curCmdSet.ChildCommands {
			if len(name) > maxLen {
				maxLen = len(name)
			}
		}

		// Sort subcommand names
		subNames := make([]string, 0, len(curCmdSet.ChildCommands))
		for name := range curCmdSet.ChildCommands {
			subNames = append(subNames, name)
		}
		sort.Strings(subNames)

		cmdIndent := strings.Repeat(" ", baseIndentSpaces+2)
		for _, name := range subNames {
			sub := curCmdSet.ChildCommands[name]
			padding := strings.Repeat(" ", maxLen-len(name)+2)
			fmt.Printf("%s%s%s - %s\n", cmdIndent, name, padding, sub.Description)
		}
		fmt.Println()
	}

	// Flag
	printFlagOptions(fs, baseIndentSpaces)

	// Top-level trailer
	if curCmdSet == rootCmd {
		fmt.Print(helpMenuTrailer)
	}
}

// Custom printer to deduplicate short/long usages and indent automatically
func printFlagOptions(fs *flag.FlagSet, baseIndentSpaces int) {
	const shortArgPrefix string = "-"      // like "  [-]t, --test  Some usage text"
	const shortLongArgJoiner string = ", " // like "  -t[, ]--test  Some usage text"
	const longArgPrefix string = "--"      // like "  -t, [--]test  Some usage text"
	const argToUsageSpaces int = 2         // like "  -t, --test[  ]Some usage text"

	type optInfo struct {
		names      []string
		usage      string
		defaultVal string
		hasShort   bool
	}

	seen := make(map[string]*optInfo)

	// Deduplicate usages by exact usage text match
	fs.VisitAll(func(arg *flag.Flag) {
		name := arg.Name
		var shortArgName, longArgName string
		if len(name) == 1 {
			shortArgName = name
		} else {
			longArgName = name
		}

		usageText := arg.Usage

		hasShort := shortArgName != ""

		// Add formatted arg text
		usage, seenUsage := seen[usageText]
		if seenUsage {
			if shortArgName != "" {
				usage.names = append(usage.names, shortArgPrefix+shortArgName)
				usage.hasShort = true
			}
			if longArgName != "" {
				usage.names = append(usage.names, longArgPrefix+longArgName)
			}
		} else {
			names := []string{}
			if shortArgName != "" {
				names = append(names, shortArgPrefix+shortArgName)
			}
			if longArgName != "" {
				names = append(names, longArgPrefix+longArgName)
			}
			seen[usageText] = &optInfo{
				names:      names,
				usage:      arg.Usage,
				defaultVal: arg.DefValue,
				hasShort:   hasShort,
			}
		}
	})

	// Deduplicated option list
	opts := []*optInfo{}
	for _, opt := range seen {
		opts = append(opts, opt)
	}

	// Ensure short args come before long args
	for _, opt := range seen {
		if len(opt.names) <= 1 {
			continue
		}

		sort.Slice(opt.names, func(indexA, indexB int) bool {
			flagNameA := opt.names[indexA]
			flagNameB := opt.names[indexB]

			return len(flagNameA) < len(flagNameB)
		})
	}

	// Sort list to group long/short args
	sort.Slice(opts, func(indexA, indexB int) bool {
		flagA := opts[indexA]
		flagB := opts[indexB]

		firstNameA := strings.ToLower(flagA.names[0])
		firstNameB := strings.ToLower(flagB.names[0])

		return firstNameA < firstNameB
	})

	// accounts for short arg prefix length, short arg default len (1), and joiner length
	longShortArgOffset := len(shortLongArgJoiner) + len(shortArgPrefix) + 1

	// Calculate max length flags for alignment
	maxLen := 0
	for _, opt := range opts {
		left := strings.Join(opt.names, shortLongArgJoiner)
		if !opt.hasShort {
			leftLen := len(left) + longShortArgOffset
			if leftLen > maxLen {
				maxLen = leftLen
			}
		} else {
			if len(left) > maxLen {
				maxLen = len(left)
			}
		}
	}

	// Print option list
	fmt.Printf("%sOptions:\n", strings.Repeat(" ", baseIndentSpaces))
	for _, opt := range opts {
		left := strings.Join(opt.names, shortLongArgJoiner)

		// Indent based on short/long
		indentSpaces := baseIndentSpaces
		if !opt.hasShort {
			indentSpaces += longShortArgOffset
		}
		indent := strings.Repeat(" ", indentSpaces)

		// Padding for this line to offset usage text
		leftLen := len(left) + (0)
		if !opt.hasShort {
			leftLen += longShortArgOffset
		}
		paddingSpaces := maxLen - leftLen + argToUsageSpaces
		if paddingSpaces < argToUsageSpaces {
			paddingSpaces = argToUsageSpaces
		}
		padding := strings.Repeat(" ", paddingSpaces)

		// Skip printing any "empty" defaults
		desc := opt.usage
		if opt.defaultVal != "" && opt.defaultVal != "false" && opt.defaultVal != "0" {
			desc += fmt.Sprintf(" [default: %s]", opt.defaultVal)
		}

		fmt.Printf("%s%s%s%s\n", indent, left, padding, desc)
	}

}

// Checks if subcommand is an immediate child of the command name in the tree
func IsValidSubcommand(root *CommandSet, cmdName, subCmdName string) (valid bool) {
	var stack []*CommandSet
	stack = append(stack, root)

	for len(stack) > 0 {
		// Pop the last element
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if current.CommandName == cmdName {
			// Found the command, return sub command presence
			_, valid = current.ChildCommands[subCmdName]
			return
		}

		// Add children to the stack for further searching
		for _, child := range current.ChildCommands {
			stack = append(stack, child)
		}
	}

	// command name not found
	return
}

// Returns the names of immediate children of cmdName, sorted alphabetically
func GetImmediateChildren(root *CommandSet, cmdName string) (subcommands []string) {
	var stack []*CommandSet
	stack = append(stack, root)

	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if current.CommandName == cmdName {
			// Collect and sort child names
			for name := range current.ChildCommands {
				subcommands = append(subcommands, name)
			}
			sort.Strings(subcommands)
			return
		}

		// Add children to stack for further search
		for _, child := range current.ChildCommands {
			stack = append(stack, child)
		}
	}

	// Command not found, return empty slice
	return
}

// Traverses the command tree strictly by lineage.
// Returns the matched command, its parent stack, and an error if the path is invalid.
func ResolveCommand(args []string, root *CommandSet) (subcmd *CommandSet, parents []*CommandSet, err error) {
	subcmd = root
	parents = []*CommandSet{}

	for _, arg := range args {
		if arg == "" || arg == "root" {
			// Root command is a given
			continue
		}
		child, ok := subcmd.ChildCommands[arg]
		if ok {
			parents = append(parents, subcmd)
			subcmd = child
		} else {
			err = fmt.Errorf("unknown command: %s", arg)
			return
		}
	}
	return
}
