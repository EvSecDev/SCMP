// Package for any lower-level network related code
package network

import (
	"context"
	"fmt"
	"net"
	"scmp/internal/logctx"
)

// Checks for active network interfaces (can't deploy to remote endpoints if no network)
func LocalSystemChecks(ctx context.Context) (err error) {
	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "Running local system checks...\n")
	logctx.LogEvent(ctx, logctx.VerbosityProgress, logctx.InfoLog, "  Ensuring system has an active network interface\n")

	// Get list of local systems network interfaces
	systemNetInterfaces, err := net.Interfaces()
	if err != nil {
		err = fmt.Errorf("failed to obtain system network interfaces: %w", err)
		return
	}

	// Ensure system has an active network interface
	var noActiveNetInterface bool
	for _, iface := range systemNetInterfaces {
		// Net interface is up
		if iface.Flags&net.FlagUp != 0 {
			noActiveNetInterface = false
			break
		}
		noActiveNetInterface = true
	}
	if noActiveNetInterface {
		err = fmt.Errorf("no active network interfaces found, will not attempt network connections")
		return
	}

	return
}
