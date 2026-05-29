// Package for building/testing/linting main go program
package build

import (
	"os"
	"path/filepath"

	"golang.org/x/term"
)

func (opts *Options) RunBuilder() (exitCode int) {
	var err error

	ctx := &context{
		cliOpts: *opts,
	}

	// When tty is present, add coloring
	if term.IsTerminal(int(os.Stdout.Fd())) {
		colorRed = ansiiRed
		colorGreen = ansiiGreen
		colorYellow = ansiiYellow
		colorBlue = ansiiBlue
		colorBold = ansiiBold
		noColor = ansiiColorreset
	}
	// Otherwise, color variable stays empty with no tty

	// Setup
	ctx.repositoryRoot, err = getRepoRoot()
	if err != nil {
		printErr(0, "%w", err)
		return 1
	}
	err = os.Chdir(ctx.repositoryRoot)
	if err != nil {
		printErr(0, "Could not move into repository root: %w", err)
		return
	}
	err = loadConfig(ctx)
	if err != nil {
		printErr(0, "%w", err)
		return 1
	}

	switch {
	case opts.PrepareRelease:
		// Override options for release
		ctx.cliOpts.FullTests = true

		err = preBuildChecks(ctx)
		if err != nil {
			printErr(0, "Failed pre-build checks: %w", err)
			return 1
		}
		err = checkPackageLicenses(ctx, false)
		if err != nil {
			printErr(0, "Failed License Check: %w", err)
			return 1
		}
		err = runTests(ctx)
		if err != nil {
			printErr(0, "Failed tests: %w", err)
			return 1
		}
		err = compile(ctx, true)
		if err != nil {
			printErr(0, "Failed compilation: %w", err)
			return 1
		}
		var tempReleaseDir string
		tempReleaseDir, err = prepareReleaseFiles(ctx)
		if err != nil {
			printErr(0, "Failed Pre-Release [Files]: %w", err)
			return 1
		}
		err = generateThirdPartLicenses(filepath.Join(tempReleaseDir, "THIRD_PARTY_LICENSES.txt"))
		if err != nil {
			printErr(0, "Failed Pre-Release [Licenses]: %w", err)
			return 1
		}
		err = prepareReleaseChangelog(ctx, tempReleaseDir)
		if err != nil {
			printErr(0, "Failed Pre-Release [Changelog]: %w", err)
			return 1
		}
	case opts.PublishVersion != "":
		err = publishRelease(ctx)
		if err != nil {
			printErr(0, "Failed to Publish Release: %w", err)
			return 1
		}
	case opts.PrintLicenses:
		err = checkPackageLicenses(ctx, true)
		if err != nil {
			printErr(0, "Failed License Check: %w", err)
			return 1
		}
	case opts.Updatepackages:
		err = checkPackageLicenses(ctx, true)
		if err != nil {
			printErr(0, "Failed License Check: %w", err)
			return 1
		}
		err = updateGoPackages()
		if err != nil {
			printErr(0, "Update Aborted: %w", err)
			return 1
		}
	case opts.PrintDepTree:
		err = printDependencyTree()
		if err != nil {
			return 1
		}
	case opts.Build:
		err = preBuildChecks(ctx)
		if err != nil {
			printErr(0, "Failed pre-build checks: %w", err)
			return 1
		}
		if !opts.SkipTests {
			err = runTests(ctx)
			if err != nil {
				printErr(0, "Failed tests: %w", err)
				return 1
			}
		}
		err = compile(ctx, false)
		if err != nil {
			printErr(0, "Failed compilation: %w", err)
			return 1
		}
	default:
		printErr(0, "Insufficient, unknown, or invalid combination of options. Use -h/--help to see valid options.")
		return 1
	}
	return 0
}
