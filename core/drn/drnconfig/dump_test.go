package drnconfig

import (
	"os"
	"path/filepath"
	"scmp/core/drn"
	"scmp/internal/tests/utils"
	"strings"
	"testing"
)

func TestShowAll(t *testing.T) {
	tests := []struct {
		name          string
		filesToCreate []struct {
			rel  string
			data []byte
		}
		expectError    string
		expectContains []string
	}{
		{
			name: "Valid directory with one config file",
			filesToCreate: []struct {
				rel  string
				data []byte
			}{
				{drn.ExternalVariableDirectory + "/ns1", []byte(`{"key":"vsal"}`)},
			},
			expectError:    "",
			expectContains: []string{"Internal DRNs:", "External DRNs (located in " + drn.ExternalVariableDirectory + "):", "Macro Name -"},
		},
		{
			name: "Valid directory with multiple config files",
			filesToCreate: []struct {
				rel  string
				data []byte
			}{
				{drn.ExternalVariableDirectory + "/ns1", []byte(`{"a":"1"}`)},
				{drn.ExternalVariableDirectory + "/ns2/sub", []byte(`{"b":"2"}`)},
			},
			expectError:    "",
			expectContains: []string{"Internal DRNs:", "External DRNs (located in " + drn.ExternalVariableDirectory + "):"},
		},
		{
			name:          "Empty external config directory",
			filesToCreate: nil,
			expectError:   "failed walking configuration directory",
		},
		{
			name: "Config file with invalid JSON",
			filesToCreate: []struct {
				rel  string
				data []byte
			}{
				{drn.ExternalVariableDirectory + "/bad", []byte(`{not json}`)},
			},
			expectError:    "parse 'bad': invalid character",
			expectContains: nil,
		},
	}

	// For "directory not found" case we skip creating _global entirely
	t.Run("Config directory doesn't exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		_, err := ShowAll(tmpDir)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "no such file") {
			t.Errorf("error = %q, expected 'no such file' variant", err.Error())
		}
	})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			for _, f := range test.filesToCreate {
				target := filepath.Join(tmpDir, f.rel)
				parent := filepath.Dir(target)
				err := os.MkdirAll(parent, 0700)
				if err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				err = os.WriteFile(target, f.data, 0600)
				if err != nil {
					t.Fatalf("writefile: %v", err)
				}
			}

			table, err := ShowAll(tmpDir)
			matches, err := utils.MatchErrorString(err, test.expectError)
			if err != nil {
				t.Fatalf("%v", err)
			} else if matches {
				return
			}

			for _, s := range test.expectContains {
				if !strings.Contains(table, s) {
					t.Errorf("table missing expected substring %q\nfull table:\n%s", s, table)
				}
			}
		})
	}
}
