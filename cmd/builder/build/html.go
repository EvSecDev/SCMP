package build

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

func lintCSSAndHTML(ctx *context) (err error) {
	webStaticFilesDir := filepath.Join(ctx.repositoryRoot, "web", "static-files")
	cmd := exec.Command("biome", "lint",
		"--diagnostic-level=warn",
		"--error-on-warnings",
		webStaticFilesDir+"/",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("biome css/html: %w: %s", err, string(out))
		return
	}
	return
}
