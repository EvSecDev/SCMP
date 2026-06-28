package drnconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"scmp/core/drn"
	"scmp/internal/fsops"
	"scmp/internal/str"
	"slices"
	"strings"
)

// Converts a dotted field keyed-map and value to a recursive CfgNode
func MapToCfgNode(values map[string]string) (node CfgNode) {
	node = make(CfgNode)

	for dottedPath, value := range values {
		fields := strings.Split(dottedPath, drn.FieldSeparator)
		current := node

		for j, field := range fields {
			isLast := j == len(fields)-1

			if isLast {
				current[field] = CfgValue{
					kind: kindString,
					str:  value,
				}
				break
			}

			existing, fieldExists := current[field]
			if !fieldExists {
				child := make(CfgNode)
				current[field] = CfgValue{
					kind: kindObject,
					obj:  child,
				}
				current = child
				continue
			}

			if existing.kind != kindObject {
				break
			}

			current = existing.obj
		}
	}

	return
}

// Loads the DRN configuration file referenced in DRC namespace
func LoadConfig(repositoryPath string, fileReader fsops.FileReader, namespace []string) (cfg *CfgNode, err error) {
	pathElements := []string{drn.ExternalVariableDirectory}
	pathElements = append(pathElements, namespace...)
	relativeConfgPath := str.LocalRepoPath(filepath.Join(pathElements...))

	if fileReader == nil {
		fileReader = fsops.NewFileSystemReader(repositoryPath)
	}

	var rawConfig []byte
	rawConfig, err = fileReader(relativeConfgPath)
	if err != nil {
		err = fmt.Errorf("config load: %w", err)
		return
	}

	if len(rawConfig) == 0 {
		err = fmt.Errorf("config file '%s' is referenced but empty", relativeConfgPath)
		return
	}

	cfg = &CfgNode{}
	err = json.Unmarshal(rawConfig, cfg)
	if err != nil {
		err = fmt.Errorf("config parse: %w", err)
		return
	}
	return
}

// Takes a top level node, marshals and writes to specified file.
// Path should be the absolute path to the file.
// Will create the parent directories if not present
func (node *CfgNode) WriteConfig(path string) (err error) {
	newJSONCfg, err := json.MarshalIndent(node, "", "  ")
	if err != nil {
		err = fmt.Errorf("parse: %w", err)
		return
	}

	configDirectory := filepath.Dir(path)
	_, err = os.Stat(configDirectory)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		err = fmt.Errorf("config directory: %w", err)
		return
	} else if errors.Is(err, os.ErrNotExist) {
		err = os.MkdirAll(configDirectory, 0700)
		if err != nil {
			err = fmt.Errorf("failed creating config directory: %w", err)
			return
		}
	}

	err = os.WriteFile(path, newJSONCfg, 0600)
	if err != nil {
		err = fmt.Errorf("file write: %w", err)
		return
	}
	return
}

// Looks up a DRN in a loaded external config
func (node *CfgNode) LookupValue(fields []string) (value str.DRNVal, err error) {
	currentNode := *node // Starting at top level node
	for depth, fieldName := range fields {
		newNode, ok := currentNode[fieldName]
		if !ok {
			err = fmt.Errorf("field '%s' not found at depth %d in config", fieldName, depth)
			return
		}

		// Final element
		if depth == len(fields)-1 {
			// Must be a string
			if newNode.kind != kindString {
				err = fmt.Errorf("field '%s' must be value of type string (got object)", fieldName)
				return
			}
			value = str.DRNVal(newNode.str)
			break
		}

		// Middle element, must be object
		if newNode.kind != kindObject {
			err = fmt.Errorf("field '%s' at depth %d is a string, but expected an object", fieldName, depth)
			return
		}

		// Continue to next node
		currentNode = newNode.obj
	}
	return
}

// Adds/Overwrites a new value to the DRN field in the config object
func (node *CfgNode) InsertValue(fields []string, newValue str.DRNVal) (err error) {
	if *node == nil {
		// Initialize the underlying map
		*node = make(CfgNode)
	}

	current := *node
	for depth, field := range fields {
		atLastField := depth == len(fields)-1

		if atLastField {
			// Ensure current map value is a value and not more objects
			mapVal := current[field]
			if mapVal.kind == kindObject {
				err = fmt.Errorf("field %d (%s) expected final field to be string but found object", depth, field)
				return
			}

			// Overwrite final value with new value
			current[field] = CfgValue{
				kind: kindString,
				str:  string(newValue),
			}
			return
		}

		existing, fieldExists := current[field]
		if !fieldExists {
			// Create new sub-map
			child := make(CfgNode)

			// Save new sub-map to current map
			current[field] = CfgValue{
				kind: kindObject,
				obj:  child,
			}

			// Move into sub-map
			current = child
			continue
		}

		// At intermediate level, ensure only objects
		if existing.kind != kindObject {
			err = fmt.Errorf("field %d (%s) expected object but found string", depth, field)
			return
		}

		current = existing.obj
	}

	return
}

// Retrieves all values in the config node (all levels) and formats as a DRN (validating as well).
// Returned map is keyed on the DRN string and value of the DRN value in the config (not resolved)
func (node *CfgNode) FormatAll(path string) (allDRNs map[str.DRN]str.DRNVal, err error) {
	if node == nil {
		return
	}
	if *node == nil {
		// Empty map
		return
	}

	allDRNs = make(map[str.DRN]str.DRNVal)

	pathSegments := strings.Split(path, string(os.PathSeparator))
	relativePathStart := slices.Index(pathSegments, drn.ExternalVariableDirectory)
	if relativePathStart == -1 {
		err = fmt.Errorf("path provided does not contain %s", drn.ExternalVariableDirectory)
		return
	}
	baseDRNnamespace := pathSegments[relativePathStart+1:]

	results := make(map[string]str.DRNVal)
	visitNode(*node, []string{}, results)

	for fields, value := range results {
		if fields == "" {
			continue
		}
		var newDRC drn.DRC
		newDRC.Namespace = baseDRNnamespace
		newDRC.Fields = strings.Split(string(fields), drn.FieldSeparator)

		err = newDRC.SerializeExpanded()
		if err != nil {
			err = fmt.Errorf("failed verification for fields %v: %w", fields, err)
			return
		}

		err = validateConcreteDRN(newDRC.Expanded)
		if err != nil {
			return
		}

		allDRNs[newDRC.Expanded] = value
	}

	return
}

func visitNode(node CfgNode, fields []string, collectionBucket map[string]str.DRNVal) {
	for fieldName, value := range node {
		// Make a copy of the path for this branch
		newPath := append([]string{}, fields...)
		newPath = append(newPath, fieldName)

		switch value.kind {
		case kindString:
			// String: record the fields as one string
			pathKey := strings.Join(newPath, drn.FieldSeparator)
			collectionBucket[pathKey] = str.DRNVal(value.str)
		case kindObject:
			// Object: recurse
			visitNode(value.obj, newPath, collectionBucket)
		}
	}
}

func validateConcreteDRN(input str.DRN) (err error) {
	if drn.ContainsMacro(input) {
		// Policy decision: a config path and/or config fields must always be concrete.
		// Dynamic expansion is never done against a literal path or a literal dotted field.
		// A DRN value can be a drn with macros. And a DRN inside an actual repo file can contain macros.
		lerr := fmt.Errorf("concrete drn '%s' contains macros", input)
		err = fmt.Errorf("%w: DRN literals (namespaces and fields) for config files cannot be dynamic", lerr)
		return
	}
	return
}
