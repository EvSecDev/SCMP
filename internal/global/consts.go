// Package for any constants used across the entire program
package global

type CtxKey string

const (
	ProgVersion string = "v6.0.0-alpha.8"

	GlobalUsername string = "_global"

	// Context keys
	UserKey  CtxKey = "user"        // username
	EmailKey CtxKey = "email"       // email address
	IDKey    CtxKey = "id"          // Request Tracking Identifier
	PermKey  CtxKey = "permissions" // Users configured permissions
	ConfKey  CtxKey = "config"      // Required configurations for the user
	OpsKey   CtxKey = "options"     // Optional parameters defined by user

	// Local
	FileURIPrefix         string = "file://" // Used by the user to tell certain arguments to load file content
	MaxDirectoryLoopCount int    = 200       // Maximum recursion for any loop over directories
)
