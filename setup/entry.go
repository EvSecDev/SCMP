// Package for installation actions
package setup

import (
	"embed"
)

// Read in installation static files at compile time
//
//go:embed static-files/apparmor-profile.config
//go:embed static-files/default-ssh.config
//go:embed static-files/autocomplete.sh
var installationConfigs embed.FS
