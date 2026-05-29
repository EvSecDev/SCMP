package internal

import (
	"fmt"
	"os"
	"scmp/internal/crypto"
	"scmp/internal/fsops"
	"strings"

	"gopkg.in/yaml.v2"
)

func (webConf *WebConfig) ExtractWebOptions(configFilePath string) (err error) {
	configFilePath, err = fsops.ExpandHomeDirectory(configFilePath)
	if err != nil {
		err = fmt.Errorf("failed to resolve absolute path for '%s': %w", configFilePath, err)
		return
	}

	data, err := os.ReadFile(configFilePath)
	if err != nil {
		err = fmt.Errorf("failed reading %s: %w", configFilePath, err)
		return
	}

	err = yaml.Unmarshal(data, &webConf)
	if err != nil {
		err = fmt.Errorf("failed parsing %s: %w", configFilePath, err)
		return
	}

	webConf.HTTP.TLSCertFile, err = fsops.ExpandHomeDirectory(webConf.HTTP.TLSCertFile)
	if err != nil {
		err = fmt.Errorf("failed to resolve absolute path for '%s': %w", webConf.HTTP.TLSCertFile, err)
		return
	}
	webConf.HTTP.TLSKeyFile, err = fsops.ExpandHomeDirectory(webConf.HTTP.TLSKeyFile)
	if err != nil {
		err = fmt.Errorf("failed to resolve absolute path for '%s': %w", webConf.HTTP.TLSKeyFile, err)
		return
	}

	if webConf.HTTP.ListenPort == 0 {
		webConf.HTTP.ListenPort = HTTPListenPort
	}

	// Validate existing users
	seenUsers := make(map[string]bool)
	for _, user := range webConf.UserCfg.Users {
		if seenUsers[user.Username] {
			err = fmt.Errorf("username '%s' is duplicate and already exists in configuration file", user.Username)
			return
		}

		// Reject names using internal prefix
		if strings.HasPrefix(user.Username, "_") {
			err = fmt.Errorf("user %s has illegal name: usernames cannot start with underscore", user.Username)
			return
		}

		if !crypto.IsValidUsername(user.Username) {
			err = fmt.Errorf("user %s has invalid name: must be 3 to 32 characters in length and have only alphanumeric characters", user.Username)
			return
		}

		seenUsers[user.Username] = true
	}

	err = WOAuthConfig(webConf.UserCfg)
	if err != nil {
		err = fmt.Errorf("failed to set global user config: %w", err)
		return
	}

	err = WORepoConfig(webConf.RepoCfg)
	if err != nil {
		err = fmt.Errorf("failed to set global repo config: %w", err)
		return
	}

	return
}

// Create new RPC error object
func (err *Error) New(code int, msg string, details string) {
	err.Code = code
	err.Message = msg
	if details != "" {
		err.Data = details
	}
}
