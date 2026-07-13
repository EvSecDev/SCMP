package build

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"scmp/cmd/builder/build/helpers"
	"slices"
	"strings"
)

func checkPackageLicenses(ctx *context, verboseMode bool) (err error) {
	printInfo(0, "Checking licenses for Go dependencies...")

	modules, err := getModuleList()
	if err != nil {
		err = fmt.Errorf("module retrieval failed: %w", err)
		return
	}

	const baseSpacing int = 75

	var foundDisallowed bool
	for _, module := range modules {
		if module == "" {
			continue
		}
		var licenseName string
		var license, warning string
		license, warning, err = getModuleLicense(module)
		if err != nil {
			err = fmt.Errorf("module %s: %w", module, err)
			return
		} else if warning != "" {
			printWarn(2, "module %s: %s", module, warning)
			licenseName = "UNKNOWN"
		} else {
			// Got a license, extract name
			licenseName = extractLicenseName(license)
		}

		spaceDelimiterLen := baseSpacing - len(module)

		// Policy enforcement
		isPermitted := slices.Contains(ctx.cfg.License.Permitted, licenseName)
		isDisallowed := slices.Contains(ctx.cfg.License.Disallowed, licenseName)
		if isDisallowed {
			fmt.Printf("  %s[-] DISALLOWED%s   : %s%s- %s\n",
				colorRed, noColor, module, strings.Repeat(" ", spaceDelimiterLen), licenseName)
			foundDisallowed = true
		} else if isPermitted {
			if verboseMode {
				fmt.Printf("  %s[+] VALID     %s   : %s%s- %s\n",
					colorGreen, noColor, module, strings.Repeat(" ", spaceDelimiterLen), licenseName)
			}
		} else {
			fmt.Printf("  %s[?] UNCLASSIFIED%s : %s%s- %s\n",
				colorYellow, noColor, module, strings.Repeat(" ", spaceDelimiterLen), licenseName)
			foundDisallowed = true
		}
	}
	if foundDisallowed {
		err = fmt.Errorf("compliance check FAILED")
		return
	}
	printSuccess(0, "Done")
	return
}

func getModuleList() (modules []string, err error) {
	cmd := exec.Command("go", "list", "-m", "-json", "all")
	var out []byte
	out, err = cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("go list: %w: %s", err, string(out))
		return
	}

	dec := json.NewDecoder(bytes.NewReader(out))

	var modInfos []goDownloadJSON
	for {
		var modInfo goDownloadJSON
		err = dec.Decode(&modInfo)
		if err == io.EOF {
			err = nil
			break
		}
		if err != nil {
			err = fmt.Errorf("failed to parse go list JSON object: %w", err)
			return
		}
		modInfos = append(modInfos, modInfo)
	}

	for _, modInfo := range modInfos {
		if modInfo.Main {
			continue
		}
		modules = append(modules, modInfo.Path+"@"+modInfo.Version)
	}
	return
}

func getModuleLicense(moduleName string) (license string, warning string, err error) {
	if moduleName == "" {
		err = fmt.Errorf("cannot get license for empty module name")
		return
	}

	cmd := exec.Command("go", "mod", "download", "-json", moduleName)
	var out []byte
	out, err = cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("go list: %w: %s", err, string(out))
		return
	}

	var modInfo goDownloadJSON
	err = json.Unmarshal(out, &modInfo)
	if err != nil {
		err = fmt.Errorf("failed parsing module info JSON: %w", err)
		return
	}

	_, err = os.Stat(modInfo.Dir)
	if errors.Is(err, os.ErrNotExist) {
		err = fmt.Errorf("downloaded source directory '%s' does not exist", modInfo.Dir)
		return
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		err = fmt.Errorf("failed to check downloaded source directory '%s': %w", modInfo.Dir, err)
		return
	}

	licenseFile, warning, err := findLicenseFile(modInfo.Dir)
	if err != nil {
		return
	} else if warning != "" {
		return
	}

	licenseBytes, err := os.ReadFile(licenseFile)
	if err != nil {
		err = fmt.Errorf("failed to read license file '%s': %w", licenseFile, err)
		return
	}
	if len(licenseBytes) == 0 {
		warning = fmt.Sprintf("license for module %s is empty (path: %s)", moduleName, licenseFile)
		return
	}
	license = string(licenseBytes)
	return
}

func findLicenseFile(moduleDirectory string) (licenseFilePath string, warning string, err error) {
	if moduleDirectory == "" {
		err = fmt.Errorf("cannot find license file for empty module directory path")
		return
	}

	moduleDirectory = filepath.Clean(moduleDirectory)

	const maxDepth int = 4
	rootDepth := strings.Count(moduleDirectory, string(os.PathSeparator))

	var matchingLicenseFiles []string

	err = filepath.Walk(moduleDirectory, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info == nil {
			return nil
		}
		depth := strings.Count(filepath.Clean(path), string(filepath.Separator)) - rootDepth
		if info.IsDir() || depth >= maxDepth {
			return nil
		}

		fileName := strings.ToLower(filepath.Base(path))
		if strings.HasPrefix(fileName, "license") || strings.HasPrefix(fileName, "copying") {
			matchingLicenseFiles = append(matchingLicenseFiles, path)
		}
		return nil
	})
	if err != nil {
		err = fmt.Errorf("failed walking module directory: %w", err)
		return
	}

	if len(matchingLicenseFiles) == 0 {
		warning = fmt.Sprintf("could not identify a license file by name in source module directory '%s'", moduleDirectory)
		return
	}

	licenseFilePath = matchingLicenseFiles[0] // First found wins
	return
}

func extractLicenseName(licenseText string) (name string) {
	simpliedText := strings.ToLower(licenseText)
	simpliedText = strings.Join(strings.Fields(simpliedText), " ")

	switch {
	case strings.Contains(simpliedText, "mit license"):
		name = "MIT"
		return
	case strings.Contains(simpliedText, "bsd 2-clause"):
		name = "BSD-2-Clause"
		return
	case strings.Contains(simpliedText, "bsd 3-clause"):
		name = "BSD-3-Clause"
		return
	case strings.Contains(simpliedText, "apache"):
		name = "Apache-2.0"
		return
	case strings.Contains(simpliedText, "lesser general public"):
		name = "LGPL"
		return
	case strings.Contains(simpliedText, "general public license"):
		name = "GPL"
		return
	case strings.Contains(simpliedText, "affero"):
		name = "AGPL"
		return
	case strings.Contains(simpliedText, "mozilla public"):
		name = "MPL"
		return
	case strings.Contains(simpliedText, "isc license"):
		name = "ISC"
		return
	default:
		name = "UNKNOWN"
	}

	// More intense pattern matching when not found

	// MIT may not have header
	mitIndicator := "copyright "
	if strings.HasPrefix(simpliedText, mitIndicator) {
		mitTokens := []string{
			"permission is hereby granted, free of charge",
			`the software is provided "as is"`,
		}
		if helpers.ContainsAll(simpliedText, mitTokens) {
			name = "MIT"
			return
		}
	}

	bsd3Tokens := []string{
		"redistribution and use in source and binary forms",
		"neither the name of",
		"may be used to endorse or promote",
	}
	if helpers.ContainsAll(simpliedText, bsd3Tokens) {
		name = "BSD-3-Clause"
		return
	}

	bsd2Tokens := []string{
		"redistribution and use in source and binary forms",
		"redistributions in binary form must reproduce",
		`this software is provided by the copyright holders and contributors "as is"`,
	}
	if helpers.ContainsAll(simpliedText, bsd2Tokens) {
		name = "BSD-2-Clause"
		return
	}

	return
}

func generateThirdPartLicenses(outputFile string) (err error) {
	if outputFile == "" {
		err = fmt.Errorf("output file for licenses not specified")
		return
	}
	printInfo(0, "Generating third party license file in '%s'...", outputFile)

	const header string = `THIRD PARTY LICENSES

This distribution includes third-party software components.
`

	output, err := os.OpenFile(outputFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		err = fmt.Errorf("failed to open output file: %w", err)
		return
	}

	_, err = output.WriteString(header)
	if err != nil {
		err = fmt.Errorf("failed to write header to output: %w", err)
		return
	}

	licenseDedup := make(map[string]string)

	modules, err := getModuleList()
	if err != nil {
		err = fmt.Errorf("module retrieval failed: %w", err)
		return
	}

	var encounteredError bool
	for _, module := range modules {
		if module == "" {
			continue
		}
		var licenseName string
		var license, warning string
		license, warning, err = getModuleLicense(module)
		if err != nil {
			err = fmt.Errorf("module %s: %w", module, err)
			return
		} else if warning != "" {
			printWarn(2, "module %s: %s", module, warning)
			encounteredError = true
			continue
		} else {
			// Got a license, extract name
			licenseName = extractLicenseName(license)
		}

		moduleNameFields := strings.Split(module, "@")
		if len(moduleNameFields) != 2 {
			printWarn(2, "Invalid formatted module name '%s'", module)
			continue
		}
		name := moduleNameFields[0]
		version := moduleNameFields[1]

		// Unique ID for this license text
		hash := helpers.Hash([]byte(license))

		// Build delimited section for just this license
		var licenseSection strings.Builder
		licenseSection.WriteString("------------------------------------------------------------\n")
		fmt.Fprintf(&licenseSection, "Module: %s\n", name)
		fmt.Fprintf(&licenseSection, "Version: %s\n", version)
		fmt.Fprintf(&licenseSection, "License: %s\n", licenseName)
		fmt.Fprintf(&licenseSection, "Source: https://%s\n", name)
		licenseSection.WriteString("------------------------------------------------------------\n")
		licenseSection.WriteString("\n")

		// Only printing license text when unique, otherwise show reference
		seenModule, seenLicense := licenseDedup[hash]
		if !seenLicense {
			licenseDedup[hash] = module
			licenseSection.WriteString(license)
			licenseSection.WriteString("\n")
			fmt.Fprintf(&licenseSection, "[License ID: %s]\n", hash)
		} else {
			licenseSection.WriteString("License text identical to:\n")
			licenseSection.WriteString("  ")
			licenseSection.WriteString(seenModule)
			licenseSection.WriteString("\n")
			fmt.Fprintf(&licenseSection, "[License ID: %s]\n", hash)
		}

		licenseSection.WriteString("\n")
		licenseSection.WriteString("\n")

		_, err = output.WriteString(licenseSection.String())
		if err != nil {
			err = fmt.Errorf("failed to write license for module %s to output: %w", module, err)
			return
		}
	}
	if encounteredError {
		err = fmt.Errorf("some dependencies were skipped due to license issues")
		return
	}

	printSuccess(0, "Done")
	return
}
