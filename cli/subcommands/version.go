package subcommands

import (
	"context"
	"fmt"
	"runtime"
	"scmp/internal/global"
)

func Version(ctx context.Context, commandname string, args []string) {
	// Maintain function signature compatibility
	_ = ctx
	_ = commandname

	if len(args) > 0 && (args[0] == "--verbosity" || args[0] == "-v") {
		fmt.Printf("SCMP Controller %s\n", global.ProgVersion)
		fmt.Printf("Built using %s(%s) for %s on %s\n", runtime.Version(), runtime.Compiler, runtime.GOOS, runtime.GOARCH)
		fmt.Print("License GPLv3+: GNU GPL version 3 or later <https://gnu.org/licenses/gpl.html>\n")
	} else {
		fmt.Println(global.ProgVersion)
	}
}
