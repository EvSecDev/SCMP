package drnconfig

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"scmp/core/drn"
	glbConfig "scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/str"
	"scmp/internal/tests/utils"
	"strings"
	"testing"
)

func TestWriteNewExternal(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	cfg := glbConfig.Config{
		RepositoryPath: tmpDir,
	}
	ctx = context.WithValue(ctx, global.ConfKey, cfg)

	tests := []struct {
		name        string
		drn         string
		value       str.DRNVal
		preCreate   func(string) error // setup: write existing config before call
		expectError string
	}{
		{
			name:  "Create new config file",
			drn:   drn.QuickFormat([]string{"newns"}, "key", "val"),
			value: str.DRNVal("hello"),
		},
		{
			name:  "Overwrite existing value in existing config",
			drn:   drn.QuickFormat([]string{"existing"}, "key", "old"),
			value: str.DRNVal("replaced"),
			preCreate: func(root string) error {
				ncDir := filepath.Join(root, drn.ExternalVariableDirectory)
				err := os.MkdirAll(ncDir, 0700)
				if err != nil {
					return err
				}
				node := MapToCfgNode(map[string]string{"key" + drn.FieldSeparator + "old": "oldvalue"})
				return node.WriteConfig(filepath.Join(ncDir, "existing"))
			},
		},
		{
			name:        "Invalid DRN",
			drn:         "invalid-string",
			value:       str.DRNVal("x"),
			expectError: drn.ErrNotDRN.Error(),
		},
		{
			name:  "Nested insert creates directories",
			drn:   drn.QuickFormat([]string{"deep"}, "a", "b", "c"),
			value: str.DRNVal("deepval"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.preCreate != nil {
				err := test.preCreate(tmpDir)
				if err != nil {
					t.Fatalf("preCreate: %v", err)
				}
			}

			path, err := WriteNewExternal(ctx, test.drn, test.value)
			matches, err := utils.MatchErrorString(err, test.expectError)
			if err != nil {
				t.Fatalf("%v", err)
			} else if matches {
				return
			}

			// Verify file exists and contains the value
			if path == "" {
				t.Fatal("expected non-empty path")
			}

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("readfile: %v", err)
			}

			var node CfgNode
			err = json.Unmarshal(data, &node)
			if err != nil {
				t.Fatalf("unmarshal readback config: %v", err)
			}

			if !strings.Contains(string(data), string(test.value)) {
				t.Errorf("written config %q does not contain value %q", string(data), test.value)
			}
		})
	}
}
