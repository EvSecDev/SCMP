package drnconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"scmp/core/drn"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/str"
)

// Given a DRN and a value, writes it to the corresponding configuration file.
// Does not expand macros, will write as-is.
// Will overwrite the value if already present.
func WriteNewExternal(ctx context.Context, newDRN string, value str.DRNVal) (path string, err error) {
	cfg := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")

	drc, err := drn.Validate(newDRN)
	if err != nil {
		err = fmt.Errorf("validate: %w", err)
		return
	}

	err = validateConcreteDRN(str.DRN(drc.Original))
	if err != nil {
		return
	}

	extConfigPath := drn.NamespaceToPath(cfg.RepositoryPath, drc.Namespace)
	_, err = os.Stat(extConfigPath)

	var extConfig CfgNode
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		err = fmt.Errorf("failed to check config existence: %w", err)
		return
	} else if err == nil {
		// Load existing
		var extConfigContent []byte
		extConfigContent, err = os.ReadFile(extConfigPath)
		if err != nil {
			err = fmt.Errorf("read existing: %w", err)
			return
		}

		err = json.Unmarshal(extConfigContent, &extConfig)
		if err != nil {
			err = fmt.Errorf("parse existing config at '%s': %w", extConfigPath, err)
			return
		}
	}

	err = extConfig.InsertValue(drc.Fields, value)
	if err != nil {
		err = fmt.Errorf("config update: %w", err)
		return
	}

	err = extConfig.WriteConfig(extConfigPath)
	if err != nil {
		err = fmt.Errorf("update write: %w", err)
		return
	}
	path = extConfigPath
	return
}
