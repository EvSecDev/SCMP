package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"scmp/internal/config"
	"scmp/internal/crypto"
	"scmp/internal/global"
	"scmp/internal/input"
	"scmp/internal/logctx"
	"scmp/internal/str"
)

func modifyVault(ctx context.Context, endpointName str.RepoRootDir, vaultPath string) (err error) {
	cfg := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")
	opts := global.AssertFromContext[config.Opts](ctx, "opts", global.OpsKey, "config.Opts")

	ctx = logctx.AppendCtxTag(ctx, logctx.NSVault)

	// Ensure vault file exists, if not create it
	vaultFileMeta, err := os.Stat(vaultPath)
	if os.IsNotExist(err) {
		var vaultFile *os.File
		vaultFile, err = os.Create(vaultPath)
		if err != nil {
			return
		}
		vaultFileMeta, _ = vaultFile.Stat()
		err = vaultFile.Close()
		if err != nil {
			err = fmt.Errorf("failed to close vault file: %w", err)
			return
		}
	} else if err != nil {
		return
	}

	// Get unlock pass from user
	vaultPassword, err := input.AskUserSecret(ctx, "Enter password for vault", "")
	if err != nil {
		return
	}

	// Check if vault file already has data (size is larger than the header)
	vaultFileSize := vaultFileMeta.Size()
	if vaultFileSize > 28 {
		// Read in encrypted vault file
		var lockedVaultFile []byte
		lockedVaultFile, err = os.ReadFile(vaultPath)
		if err != nil {
			err = fmt.Errorf("failed to retrieve vault file: %w", err)
			return
		}

		// Decrypt Vault
		var unlockedVault string
		unlockedVault, err = crypto.Decrypt(lockedVaultFile, vaultPassword)
		if err != nil {
			return
		}

		// Unmarshal vault JSON into global struct
		err = json.Unmarshal([]byte(unlockedVault), &cfg.Vault)
		if err != nil {
			return
		}
	}

	_, hostExists := cfg.HostInfo[endpointName]
	if !hostExists {
		logctx.LogEvent(ctx, logctx.VerbosityStandard, logctx.InfoLog, "Warning: selected host '%s' is not defined in configuration file\n", endpointName)
	}

	// Get password from user for host
	loginUserName := cfg.HostInfo[endpointName].EndpointUser
	hostPassword, err := input.AskUserSecret(ctx, fmt.Sprintf("Enter %s password for host '%s' (leave empty to delete entry)", loginUserName, endpointName), "")
	if err != nil {
		return
	}

	// Remove password if user supplied empty password
	if len(hostPassword) == 0 {
		// Just return if host is not in vault
		_, hostExistsinVault := cfg.Vault[endpointName]
		if !hostExistsinVault {
			return
		}

		// Confirm with user before deleting vault entry
		var userResponse string
		if opts.AllowDeletions {
			userResponse = "y"
		} else {
			userResponse, err = input.AskUser(ctx, "Please type 'y' to delete vault host "+string(endpointName), "")
			if err != nil {
				return
			}
		}

		// Check if the user typed 'y' (always lower-case)
		if userResponse == "y" {
			// Remove vault entry for host
			delete(cfg.Vault, endpointName)
			return
		} else {
			fmt.Printf("Did not receive confirmation, exiting.\n")
			return
		}
	}

	// Ask again to confirm
	hostPasswordConfirm, err := input.AskUserSecret(ctx, fmt.Sprintf("Enter '%s' password for host '%s' again: ", loginUserName, endpointName), "")
	if err != nil {
		return
	}

	// Error if entered passwords are not identical
	if !bytes.Equal(hostPassword, hostPasswordConfirm) {
		err = fmt.Errorf("passwords do not match")
		return
	}

	// Modify/Add host.Password
	var credential config.Credential
	credential.LoginUserPassword = string(hostPassword)
	cfg.Vault[endpointName] = credential

	// Encrypt and write changes to vault file - return with or without error
	err = lockVault(ctx, vaultPassword, vaultPath)
	return
}

// Encrypts and writes current vault data back to vault file
func lockVault(ctx context.Context, vaultPassword []byte, vaultPath string) (err error) {
	cfg := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")

	// Marshal vault into json
	unlockedVault, err := json.Marshal(cfg.Vault)
	if err != nil {
		return
	}

	// Encrypt Vault
	lockedVault, err := crypto.Encrypt(unlockedVault, vaultPassword)
	if err != nil {
		return
	}

	// Write encrypted vault back to disk - return with or without error
	err = os.WriteFile(vaultPath, lockedVault, 0600)
	return
}

// Opens vault and retrieves password for remote host
func unlockVault(ctx context.Context, endpointName str.RepoRootDir, vaultPath string) (hostPassword string, err error) {
	cfg := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")

	logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Host requires password, unlocking vault\n")

	// Open vault if not already open - should only happen once since vault is global
	if len(cfg.Vault) == 0 {
		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Reading vault file\n")

		// Read in encrypted vault file
		var lockedVaultFile []byte
		lockedVaultFile, err = os.ReadFile(vaultPath)
		if err != nil {
			err = fmt.Errorf("failed to retrieve vault file: %w", err)
			return
		}

		// Get unlock pass from user
		var vaultPassword []byte
		vaultPassword, err = input.AskUserSecret(ctx, "Enter password for vault", "")
		if err != nil {
			return
		}

		logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Decrypting vault\n")

		// Decrypt Vault
		var unlockedVault string
		unlockedVault, err = crypto.Decrypt(lockedVaultFile, vaultPassword)
		if err != nil {
			return
		}

		// Unmarshal vault JSON using global struct
		err = json.Unmarshal([]byte(unlockedVault), &cfg.Vault)
		if err != nil {
			return
		}
	}

	logctx.LogEvent(ctx, logctx.VerbosityFullData, logctx.InfoLog, "      Retrieving password from vault\n")

	// Double check host is in vault
	_, hostHasVaultEntry := cfg.Vault[endpointName]
	if !hostHasVaultEntry {
		err = fmt.Errorf("host does not have an entry in the vault")
		return
	}

	// Retrieve password for this host
	hostPassword = cfg.Vault[endpointName].LoginUserPassword
	return
}
