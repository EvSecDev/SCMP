package build

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"scmp/cmd/builder/build/helpers"
	"strings"
)

func compileTypeScript(ctx *context) (err error) {
	err = cleanupTypeScript(ctx)
	if err != nil {
		err = fmt.Errorf("failed to reset javascript source dir: %w", err)
		return
	}

	typescriptSourceDir := filepath.Join(ctx.repositoryRoot, "ts_src")
	webStaticFilesDir := filepath.Join(ctx.repositoryRoot, "web", "static-files")
	javascriptDir := filepath.Join(webStaticFilesDir, "js")

	// Remove placeholder
	placeholderPath := filepath.Join(webStaticFilesDir, "js", "placeholder")
	err = os.Remove(placeholderPath)
	if err != nil {
		err = fmt.Errorf("failed to remove javascript placeholder: %w", err)
		return
	}

	matches, err := helpers.ScanRepo(typescriptSourceDir, false, func(path, line string) (matches bool) {
		if !strings.HasSuffix(path, ".ts") {
			return
		}
		matches = true
		return
	})
	if err != nil {
		err = fmt.Errorf("failed to get file list of typescript directory: %w", err)
		return
	}
	var allTSSrcfiles []string
	seen := make(map[string]bool)
	for _, match := range matches {
		if seen[match.Path] {
			continue
		}
		allTSSrcfiles = append(allTSSrcfiles, match.Path)
		seen[match.Path] = true
	}

	cmd := exec.Command("tsc",
		"--rootDir", typescriptSourceDir,
		"--outDir", javascriptDir,
		"--target", "ES2017",
		"--lib", "DOM,ES2017",
	)
	cmd.Args = append(cmd.Args, allTSSrcfiles...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("failed compiling typescript: %w: %s", err, string(out))
		return
	}
	return
}

func cleanupTypeScript(ctx *context) (err error) {
	webStaticFilesDir := filepath.Join(ctx.repositoryRoot, "web", "static-files")
	javascriptDir := filepath.Join(webStaticFilesDir, "js")
	placeholderPath := filepath.Join(webStaticFilesDir, "js", "placeholder")

	dirEntries, err := os.ReadDir(javascriptDir)
	if err != nil {
		err = fmt.Errorf("failed to list javascript directory: %w", err)
		return
	}

	for _, dirEntry := range dirEntries {
		itemName := dirEntry.Name()
		if dirEntry.IsDir() {
			continue
		}
		if filepath.Ext(itemName) != ".js" {
			continue
		}
		err = os.Remove(filepath.Join(javascriptDir, itemName))
		if err != nil {
			err = fmt.Errorf("failed to remove javascript file: %w", err)
			return
		}
	}

	// Put placeholder back
	err = os.WriteFile(placeholderPath, []byte("placeholder"), 0600)
	if err != nil {
		err = fmt.Errorf("failed to create placeholder file in javascript directory: %w", err)
		return
	}
	return
}
