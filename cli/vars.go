package cli

import "sync"

var (
	// Store all commands, options, arguments and their relevant help menu text
	cliOpts      *CommandSet
	cliOptsSet   bool
	cliOptsOnce  sync.Once
	cliOptsMutex sync.RWMutex
)
