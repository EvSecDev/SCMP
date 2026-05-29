package secrets

import (
	"context"
	"fmt"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/sshinternal"
)

// Writes hosts secrets (key, password) into received map
func GetHostValues(ctx context.Context, oldHostInfo config.EndpointInfo) (newHostInfo config.EndpointInfo, err error) {
	cfg := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")

	ctx = logctx.AppendCtxTag(ctx, logctx.NSVault)

	// Copy current global config for this host to local
	newHostInfo = oldHostInfo

	logctx.LogEvent(ctx, logctx.VerbosityData, logctx.InfoLog, "    Retrieving endpoint key\n")

	// Get SSH Private Key from the supplied identity file
	newHostInfo.PrivateKey, newHostInfo.KeyAlgo, err = sshinternal.IdentityToKey(ctx, newHostInfo.IdentityFile)
	if err != nil {
		err = fmt.Errorf("failed to retrieve private key: %w", err)
		return
	}

	logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Key: %d\n", newHostInfo.PrivateKey)

	// Retrieve password if required
	if newHostInfo.RequiresVault {
		newHostInfo.Password, err = unlockVault(ctx, newHostInfo.EndpointName, cfg.VaultFilePath)
		if err != nil {
			err = fmt.Errorf("error retrieving host.Password from vault: %w", err)
			return
		}

		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Password: %s\n", newHostInfo.Password)
	} else {
		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Host does not require password\n")
	}

	return
}
