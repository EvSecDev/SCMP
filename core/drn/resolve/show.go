package resolve

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/str"
)

// Returns file content after DRNs have been resolved (showing values in place)
func ShowFileResolved(ctx context.Context, path str.LocalRepoPath, hostAlias str.RepoRootDir) (output []byte, err error) {
	// Retrieve required deployment options
	cfg := global.AssertFromContext[config.Config](ctx, "config", global.ConfKey, "config.Config")

	rawContent, err := os.ReadFile(string(path))
	if err != nil {
		err = fmt.Errorf("could not read file: %w", err)
		return
	}

	replacer := NewReplacer(cfg.RepositoryPath, nil)
	replacer.ExtractDRNs(hostAlias, path, rawContent)
	err = replacer.ResolveAll(ctx, cfg.HostInfo)
	if err != nil {
		err = fmt.Errorf("resolve: %w", err)
		return
	}

	output, _, err = replacer.ReplaceDRNs(hostAlias, path, rawContent)
	if err != nil {
		err = fmt.Errorf("replace: %w", err)
		return
	}

	if !bytes.HasSuffix(output, []byte("\n")) {
		output = append(output, '\n')
	}
	return
}
