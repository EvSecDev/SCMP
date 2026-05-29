package sshinternal

const (
	DefaultConfigPath string = "~/.ssh/config"          // Default to users home directory ssh config file
	KnownHostsFile    string = "known_hosts"            // File name for ssh known hosts (same directory as ssh config)
	SSHVersionString  string = "SSH-2.0-OpenSSH_10.0p2" // Some IPS rules flag on GO's ssh client string
	MaxSSHConnections int    = 10                       // Maximum simultaneous outbound SSH connections
	MaxSSHChannels    int    = 4                        // Maximum simultaneous SSH channels per SSH connection

	// Remote
	DefaultRemoteCommandTimeout int = 10  // Time in seconds for (internal) remote command to be considered dead
	DefaultConnectTimeout       int = 30  // Time in seconds for SSH connection timeout
	DefaultCommandTimeout       int = 180 // Time in seconds for user-defined commands to be considered dead
)
