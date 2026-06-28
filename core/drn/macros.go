package drn

import (
	"fmt"
	"net"
	"scmp/internal/config"
	"scmp/internal/parsing"
	"scmp/internal/str"
	"slices"
	"strings"
)

const (
	categoryHost string = "host"
	categoryFile string = "file"
)

var registry = macroRegistry{}
var macros []macroDef = initDefinitions()

// Aligns with macroDef indices
const (
	hostAlias ContextField = iota
	hostAddress
	hostUser
	fileRepoBaseDir
	filePath
	fileName
	fileDir
)

func initDefinitions() (definition []macroDef) {
	definition = []macroDef{
		{name: InternalMacroOpen + "HOSTALIAS" + InternalMacroClose,
			category: categoryHost, contextField: hostAlias,
			drn: QuickFormat([]string{InternalNamespacePrefix}, "host", "alias"),
		},
		{name: InternalMacroOpen + "HOSTADDRESS" + InternalMacroClose,
			category: categoryHost, contextField: hostAddress,
			drn: QuickFormat([]string{InternalNamespacePrefix}, "host", "net", "address"),
		},
		{name: InternalMacroOpen + "HOSTLOGINUSER" + InternalMacroClose,
			category: categoryHost, contextField: hostUser,
			drn: QuickFormat([]string{InternalNamespacePrefix}, "host", "user"),
		},
		{name: InternalMacroOpen + "REPOBASEDIR" + InternalMacroClose,
			category: categoryFile, contextField: fileRepoBaseDir,
			drn: QuickFormat([]string{InternalNamespacePrefix}, "repo", "base", "dir"),
		},
		{name: InternalMacroOpen + "FILENAME" + InternalMacroClose,
			category: categoryFile, contextField: fileName,
			drn: QuickFormat([]string{InternalNamespacePrefix}, "repo", "file", "name"),
		},
		{name: InternalMacroOpen + "FILEPATH" + InternalMacroClose,
			category: categoryFile, contextField: filePath,
			drn: QuickFormat([]string{InternalNamespacePrefix}, "repo", "file", "path"),
		},
		{name: InternalMacroOpen + "FILEDIR" + InternalMacroClose,
			category: categoryFile, contextField: fileDir,
			drn: QuickFormat([]string{InternalNamespacePrefix}, "repo", "file", "dir"),
		},
	}
	return
}

func (registry *macroRegistry) initRegistry() {
	registry.byName = make(map[string]macroDef, len(macros))
	registry.byDRN = make(map[str.DRN]macroDef, len(macros))
	registry.host = make(map[string]struct{}, len(macros))
	registry.file = make(map[string]struct{}, len(macros))
	registry.allNames = make([]string, 0, len(macros))

	for _, macroDef := range macros {
		registry.byName[macroDef.name] = macroDef
		registry.byDRN[str.DRN(macroDef.drn)] = macroDef
		registry.allNames = append(registry.allNames, macroDef.name)

		if macroDef.category == categoryHost {
			registry.host[macroDef.name] = struct{}{}
		}
		if macroDef.category == categoryFile {
			registry.file[macroDef.name] = struct{}{}
		}
	}
}

func getRegistry() (reg *macroRegistry) {
	registry.once.Do(registry.initRegistry)
	return &registry
}

// Gets list of all internal DRNs
func GetAllInternalDRNs() (intDRNs []str.DRN) {
	registry := getRegistry()
	for drn := range registry.byDRN {
		intDRNs = append(intDRNs, drn)
	}
	slices.Sort(intDRNs)
	return
}

// Gets list of all macro names
func GetAllHostMacros() (macros []string) {
	registry := getRegistry()
	for macro := range registry.host {
		macros = append(macros, macro)
	}
	slices.Sort(macros)
	return
}

// Gets list of all macro names
func GetAllFileMacros() (macros []string) {
	registry := getRegistry()
	for macro := range registry.file {
		macros = append(macros, macro)
	}
	slices.Sort(macros)
	return
}

func HasFileMacros(parts []string) (hasFileMacros bool) {
	for _, part := range parts {
		if !strings.Contains(part, InternalMacroOpen) {
			continue
		}
		for _, macro := range macros {
			if macro.category != categoryFile {
				continue
			}
			if strings.Contains(part, macro.name) {
				hasFileMacros = true
				return
			}
		}
	}
	return
}

func HasHostMacros(parts []string) (hasHostMacros bool) {
	for _, part := range parts {
		if !strings.Contains(part, InternalMacroOpen) {
			continue
		}
		for _, macro := range macros {
			if macro.category != categoryHost {
				continue
			}
			if strings.Contains(part, macro.name) {
				hasHostMacros = true
				return
			}
		}
	}
	return
}

func IsFileMacro(macro string) (fileMacro bool) {
	registry := getRegistry()
	_, fileMacro = registry.file[macro]
	return
}

func IsHostMacro(macro string) (hostMacro bool) {
	registry := getRegistry()
	_, hostMacro = registry.host[macro]
	return
}

func IsFileDRN(drn string) (fileDRN bool) {
	registry := getRegistry()
	macroDef, validDRN := registry.byDRN[str.DRN(drn)]
	if !validDRN {
		return
	}
	if macroDef.category == categoryFile {
		fileDRN = true
	}
	return
}

func IsHostDRN(drn string) (hostDRN bool) {
	registry := getRegistry()
	macroDef, validDRN := registry.byDRN[str.DRN(drn)]
	if !validDRN {
		return
	}
	if macroDef.category == categoryHost {
		hostDRN = true
	}
	return
}

// Returns true if value contains any macro pattern {{...}}
func ContainsMacro(value str.DRN) (hasMacros bool) {
	found := ExtractMacros(string(value))
	if len(found) > 0 {
		hasMacros = true
	}
	return
}

// Looks up an internal DRN string and retrieves the formatted macro name
func InternalDRNToMacroName(intDRN str.DRN) (macroName string, isValid bool) {
	registry := getRegistry()
	def, isValid := registry.byDRN[intDRN]
	if !isValid {
		return
	}
	macroName = def.name
	return
}

func (macro macroDef) resolveHostValue(hostInfo config.EndpointInfo) (value string, err error) {
	switch macro.contextField {
	case hostAlias:
		value = string(hostInfo.EndpointName)
	case hostUser:
		value = hostInfo.EndpointUser
	case hostAddress:
		value, _, err = net.SplitHostPort(hostInfo.Endpoint)
		if err != nil {
			err = fmt.Errorf("failed extracting address from endpoint socket: %w", err)
			return
		}
	}
	if value == "" {
		err = fmt.Errorf("extracted empty value for %s macro %s from config struct %#v",
			macro.category, macro.name, hostInfo)
	}
	return
}

func (macro macroDef) resolveFileValue(repositoryPath string, path str.LocalRepoPath) (value string, err error) {
	repoBase, target := parsing.TranslateLocalPathtoRemotePath(repositoryPath, path)
	switch macro.contextField {
	case fileRepoBaseDir:
		value = string(repoBase)
	case filePath:
		value = string(target)
	case fileName:
		value = string(str.FilePathBase(target))
	case fileDir:
		value = string(str.FilePathDir(target))
	}
	if value == "" {
		err = fmt.Errorf("extracted empty value for %s macro %s from path %s/%s",
			macro.category, macro.name, repositoryPath, path)
	}
	return
}

// Use host only context to extract various pieces of information and map them to macros by name.
// Returned replacer can be used to replace a macro name with its contextual value.
func NewHostMacroReplacer(hostInfo config.EndpointInfo) (hostReplacer *strings.Replacer, err error) {
	if hostInfo.EndpointName == "" {
		lerr := fmt.Errorf("cannot build file macro replacer")
		err = fmt.Errorf("%w: missing contextual host info", lerr)
		return
	}
	var hostParts []string
	for _, macro := range macros {
		if macro.category != categoryHost {
			continue
		}

		var resolved string
		resolved, err = macro.resolveHostValue(hostInfo)
		if err != nil {
			return
		}

		hostParts = append(hostParts, macro.name, resolved)
	}
	hostReplacer = strings.NewReplacer(hostParts...)
	return
}

// Use file only context to extract various pieces of information and map them to macros by name.
// Returned replacer can be used to replace a macro name with its contextual value.
func NewFileMacroReplacer(repositoryPath string, filePath str.LocalRepoPath) (fileReplacer *strings.Replacer, err error) {
	if filePath == "" {
		lerr := fmt.Errorf("cannot build file macro replacer")
		err = fmt.Errorf("%w: missing contextual file path", lerr)
		return
	}
	var fileParts []string
	for _, macro := range macros {
		if macro.category != categoryFile {
			continue
		}

		var resolved string
		resolved, err = macro.resolveFileValue(repositoryPath, filePath)
		if err != nil {
			return
		}

		fileParts = append(fileParts, macro.name, resolved)
	}
	fileReplacer = strings.NewReplacer(fileParts...)
	return
}

// Use host and file context to extract various pieces of information and map them to macros by name.
// Returned replacers can be used to replace a macro name with its contextual value.
func NewMacroReplacer(repositoryPath string, hostInfo config.EndpointInfo, filePath str.LocalRepoPath) (fileReplacer, hostReplacer *strings.Replacer, err error) {
	fileReplacer, err = NewFileMacroReplacer(repositoryPath, filePath)
	if err != nil {
		return
	}
	hostReplacer, err = NewHostMacroReplacer(hostInfo)
	return
}

// Extracts all macro names in a string
func ExtractMacros(input string) (macros []string) {
	i := 0
	for i < len(input) {
		start := strings.Index(input[i:], InternalMacroOpen)
		if start == -1 {
			return
		}
		start += i + len(InternalMacroOpen) // absolute index, past "{{"

		end := strings.Index(input[start:], InternalMacroClose)
		if end == -1 {
			nested := strings.Index(input[start:], InternalMacroOpen)
			if nested != -1 {
				i = start + nested
				continue
			}
			macros = append(macros, InternalMacroOpen+input[start:])
			return
		}

		nested := strings.Index(input[start:start+end], InternalMacroOpen)
		if nested != -1 {
			i = start + nested
			continue
		}

		if end > 0 {
			macros = append(macros, InternalMacroOpen+input[start:start+end]+InternalMacroClose)
		}

		i = start + end + len(InternalMacroClose)
	}

	return
}
