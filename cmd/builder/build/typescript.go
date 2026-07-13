package build

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"scmp/cmd/builder/build/helpers"
	"strings"
)

func lintTypeScript(ctx *context) (err error) {
	typescriptSourceDir := filepath.Join(ctx.repositoryRoot, "ts_src")

	cmd := exec.Command("biome", "lint",
		"--diagnostic-level=warn",
		"--error-on-warnings",
		typescriptSourceDir+"/",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("biome: %w: %s", err, string(out))
		return
	}
	return
}

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
		"--strict",
		"--noUnusedLocals",
		"--noImplicitReturns",
		"--noFallthroughCasesInSwitch",
		"--noUnusedParameters",
		"--noEmitOnError",
		"--forceConsistentCasingInFileNames",
		"--exactOptionalPropertyTypes",
		"--noUncheckedIndexedAccess",
	)
	cmd.Args = append(cmd.Args, allTSSrcfiles...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("failed compiling typescript: %w: %s", err, string(out))
		warn := cleanupTypeScript(ctx)
		fmt.Printf("Warning: %v\n", warn)
		return
	}
	return
}

func cleanupTypeScript(ctx *context) (err error) {
	webStaticFilesDir := filepath.Join(ctx.repositoryRoot, "web", "static-files")
	javascriptDir := filepath.Join(webStaticFilesDir, "js")
	placeholderPath := filepath.Join(webStaticFilesDir, "js", "placeholder")

	defer func() {
		// Put placeholder back always
		err = os.WriteFile(placeholderPath, []byte("placeholder"), 0600)
		if err != nil {
			err = fmt.Errorf("failed to create placeholder file in javascript directory: %w", err)
			return
		}
	}()

	err = filepath.WalkDir(javascriptDir, func(path string, d fs.DirEntry, inErr error) (err error) {
		if inErr != nil {
			if strings.HasSuffix(inErr.Error(), "no such file or directory") {
				inErr = nil // recursive dir removal will hit sub items
			} else {
				err = inErr
			}
			return
		}
		if path == javascriptDir {
			// Do not remove root dir
			return
		}

		err = os.RemoveAll(path)
		return
	})
	if err != nil {
		err = fmt.Errorf("failed removing javascript items: %w", err)
		return
	}

	return
}
