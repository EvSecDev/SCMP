package build

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Dynamically obtain the repository root directory from current directory
func getRepoRoot() (repoRoot string, err error) {
	pwd, err := os.Getwd()
	if err != nil {
		err = fmt.Errorf("failed to get current working directory: %w", err)
		return
	}
	gitDir := filepath.Join(pwd, ".git")
	info, err := os.Stat(gitDir)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		err = fmt.Errorf("builder must be run from root of repository (%s does not exist in current working directory)", gitDir)
		return
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		err = fmt.Errorf("failed to check for existence of git directory: %w", err)
		return
	}
	if !info.IsDir() {
		err = fmt.Errorf("expected directory at '%s' but found a file instead", gitDir)
		return
	}
	repoRoot, err = filepath.Abs(pwd)
	if err != nil {
		err = fmt.Errorf("failed to get absolute path for repository root: %w", err)
		return
	}
	return
}

// Load build config
func loadConfig(ctx *context) (err error) {
	configFile, err := os.ReadFile(filepath.Join(ctx.repositoryRoot, ".build.json"))
	if err != nil {
		err = fmt.Errorf("failed to read configuration file: %w", err)
		return
	}
	err = json.Unmarshal(configFile, &ctx.cfg)
	if err != nil {
		err = fmt.Errorf("failed to parse configuration file JSON: %w", err)
		return
	}

	// Validate

	if ctx.cfg.ProgramOutputName == "" {
		err = fmt.Errorf("config option binaryShortName cannot be empty")
		return
	}
	if ctx.cfg.ProgramLongPrefix == "" {
		err = fmt.Errorf("config option binaryLongNamePrefix cannot be empty")
		return
	}

	return
}
