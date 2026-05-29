package build

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"scmp/cmd/builder/build/helpers"
	"strings"
)

func updateHTMLStaticFields(ctx *context) (err error) {
	htmlStaticDir := filepath.Join(ctx.repositoryRoot, "web", "static-files")

	// Extract program version from source
	mainConstsFile := filepath.Join(ctx.repositoryRoot, globalConstsFile)
	constsFile, err := os.ReadFile(mainConstsFile)
	if err != nil {
		err = fmt.Errorf("failed to read global consts: %w", err)
		return
	}

	versionNum, err := helpers.GetProgVersion(constsFile, versionVariableName)
	if err != nil {
		return
	}

	buildOS := strings.ToUpper(ctx.cliOpts.OperatingSystem)
	buildArch := strings.ToUpper(ctx.cliOpts.Architecture)

	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("git: %w: %s", err, string(out))
		return
	}
	headCommitHash := string(bytes.TrimSpace(bytes.Trim(out, "\n")))

	if versionNum == "" || buildOS == "" || buildArch == "" || headCommitHash == "" {
		err = fmt.Errorf("could not get all required information to update HTML")
		return
	}

	findAndReplace := map[string]string{
		`<div id="version-info">`:  "SCMP Controller " + versionNum,
		`<div id="build-info">`:    "Build: " + headCommitHash,
		`<div id="platform-info">`: "Platform: " + buildOS + " " + buildArch,
	}

	// Using scan repo for easy framework for replacements
	err = filepath.WalkDir(htmlStaticDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".html") {
			return nil
		}

		htmlFile, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		for prefix, replacement := range findAndReplace {
			startIndex := bytes.Index(htmlFile, []byte(prefix))
			if startIndex == -1 {
				continue
			}

			contentStart := startIndex + len(prefix)

			relEnd := bytes.Index(htmlFile[contentStart:], []byte(`</div>`))
			if relEnd == -1 {
				continue
			}

			contentEnd := contentStart + relEnd

			newHTML := make([]byte, 0, len(htmlFile)-(contentEnd-contentStart)+len(replacement))

			newHTML = append(newHTML, htmlFile[:contentStart]...)
			newHTML = append(newHTML, replacement...)
			newHTML = append(newHTML, htmlFile[contentEnd:]...)

			htmlFile = newHTML
		}

		err = os.WriteFile(path, htmlFile, 0600)
		return err
	})
	if err != nil {
		err = fmt.Errorf("failed updating HTML fields: %w", err)
		return
	}
	return
}
