// Package for program secrets handling (vault)
package secrets

import (
	"context"
	"fmt"
	"scmp/internal/config"
	"scmp/internal/crypto"
	"scmp/internal/input"
	"scmp/internal/logctx"
	"scmp/internal/str"
)

func CLIEntry(ctx context.Context, config config.Config, modifyVaultHost str.RepoRootDir, genNewHash bool) (err error) {
	if modifyVaultHost != "" {
		err = modifyVault(ctx, modifyVaultHost, config.VaultFilePath)
		if err != nil {
			err = fmt.Errorf("vault: %w", err)
			return
		}
	} else if genNewHash {
		var password []byte
		password, err = input.AskUserSecret(ctx, "Password", "")
		if err != nil {
			err = fmt.Errorf("failed prompting for password: %w", err)
			return
		}

		var hash string
		hash, err = crypto.HashUserPassword(string(password))
		if err != nil {
			err = fmt.Errorf("failed generating hash: %w", err)
			return
		}

		logctx.LogStdInfo(ctx, "\nHash:\n%s\n", hash)
	}
	return
}
