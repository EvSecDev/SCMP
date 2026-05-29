package internal

import "time"

const (
	HTTPListenPort       int           = 8443                // Default listen port
	HTTPListenAddr       string        = "localhost"         // Hard coded to loopback for lower attack surface
	HTTPReadTimeout      time.Duration = 30 * time.Second
	HTTPWriteTimeout     time.Duration = 90 * time.Second
	HTTPIdleTimeout      time.Duration = 900 * time.Second
	UploadPath           string        = "/data-store/upload"
	DownloadBasePath     string        = "/data-store/download/"

	NoAuthAction PermissionAction = "noauth" // Client can do it without login
	ReadAction   PermissionAction = "read"   // User can read (non-mutating)
	WriteAction  PermissionAction = "write"  // User can mutate things
	SeedAction   PermissionAction = "seed"   // User can browse remote files and initiate download
	DeployAction PermissionAction = "deploy" // User can start and view deployments
	AdminAction  PermissionAction = "admin"  // User has all of the above perms

	RepoHeaderName string = "SCMP-Repository" // HTTP header for user repo selection
)
