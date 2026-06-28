package resolve

import (
	"context"
	"os"
	"path/filepath"
	"scmp/core/drn"
	"scmp/core/drn/drnconfig"
	glbConfig "scmp/internal/config"
	"scmp/internal/global"
	"scmp/internal/logctx"
	"scmp/internal/str"
	"scmp/internal/tests/utils"
	"testing"
)

func TestResolve(t *testing.T) {
	ctx := t.Context()
	ctx = logctx.New(ctx, logctx.NSTest, logctx.VerbosityNone, ctx.Done())

	tests := []struct {
		name                 string
		origin               originKey
		hostInfo             glbConfig.EndpointInfo
		topDRNConfig         *drn.DRC
		extConfigPath        string
		extConfig            drnconfig.CfgNode
		rawFileContent       []byte   // For edge cases like empty/invalid JSON
		parentDRC            *drn.DRC // For parent propagation testing
		expect               str.DRNVal
		expectError          string
		expectParentResolved bool // Flags that parent.resolved should be checked
	}{
		{
			name:     "Single Level Internal File Name Macro",
			origin:   originKey{globalID: "host1", file: "host1/etc/file1.conf"},
			hostInfo: glbConfig.EndpointInfo{EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{drn.InternalNamespacePrefix}, "repo", "file", "name")),
				Namespace: []string{drn.InternalNamespacePrefix},
				Fields:    []string{"repo", "file", "name"},
			},
			expect: "file1.conf",
		},
		{
			name:     "Single Level Internal File Path Macro",
			origin:   originKey{globalID: "host1", file: "host1/etc/file1.conf"},
			hostInfo: glbConfig.EndpointInfo{EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{drn.InternalNamespacePrefix}, "repo", "file", "path")),
				Namespace: []string{drn.InternalNamespacePrefix},
				Fields:    []string{"repo", "file", "path"},
			},
			expect: "/etc/file1.conf",
		},
		{
			name:     "Single Level Internal Host Alias Macro",
			origin:   originKey{globalID: "host1", file: "host1/etc/file1.conf"},
			hostInfo: glbConfig.EndpointInfo{EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{drn.InternalNamespacePrefix}, "host", "alias")),
				Namespace: []string{drn.InternalNamespacePrefix},
				Fields:    []string{"host", "alias"},
			},
			expect: "host1",
		},
		{
			name:     "Single Level Internal Host Address Macro",
			origin:   originKey{globalID: "host1", file: "host1/etc/file1.conf"},
			hostInfo: glbConfig.EndpointInfo{EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{drn.InternalNamespacePrefix}, "host", "net", "address")),
				Namespace: []string{drn.InternalNamespacePrefix},
				Fields:    []string{"host", "net", "address"},
			},
			expect: "127.0.0.1",
		},
		{
			name:     "Single Level External Direct",
			origin:   originKey{globalID: "host1", file: "host1/etc/file1.conf"},
			hostInfo: glbConfig.EndpointInfo{EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"class"}, "host", "type")),
				Namespace: []string{"class"},
				Fields:    []string{"host", "type"},
			},
			extConfigPath: "class",
			extConfig:     drnconfig.MapToCfgNode(map[string]string{"host" + drn.FieldSeparator + "type": "production"}),
			expect:        "production",
		},
		{
			name:     "Multi Level External All",
			origin:   originKey{globalID: "host1", file: "host1/etc/file1.conf"},
			hostInfo: glbConfig.EndpointInfo{EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"class"}, "host", "subclass")),
				Namespace: []string{"class"},
				Fields:    []string{"host", "subclass"},
			},
			extConfigPath: "class",
			extConfig: drnconfig.MapToCfgNode(map[string]string{
				"host" + drn.FieldSeparator + "type":     drn.QuickFormat([]string{"class"}, "info", "{{HOSTALIAS}}"),
				"host" + drn.FieldSeparator + "subclass": drn.QuickFormat([]string{"class"}, "host", "type"),
				"info" + drn.FieldSeparator + "host1":    "production",
			}),
			expect: "production",
		},
		{
			name:     "External Config DRN field typo",
			origin:   originKey{globalID: "host1", file: "host1/etc/file1.conf"},
			hostInfo: glbConfig.EndpointInfo{EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"class"}, "host", "tpye")),
				Namespace: []string{"class"},
				Fields:    []string{"host", "tpye"},
			},
			extConfigPath: "class",
			extConfig:     drnconfig.MapToCfgNode(map[string]string{"host" + drn.FieldSeparator + "type": "production"}),
			expectError:   "field 'tpye' not found at depth 1 in config",
		},
		{
			name:     "Cyclic DRN Reference Detected",
			origin:   originKey{globalID: "host1", file: "host1/etc/file.conf"},
			hostInfo: glbConfig.EndpointInfo{EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"class"}, "self", "ref")),
				Namespace: []string{"class"},
				Fields:    []string{"self", "ref"},
				Expanded:  str.DRN(drn.QuickFormat([]string{"class"}, "self", "ref")),
			},
			extConfigPath: "class",
			extConfig: drnconfig.MapToCfgNode(map[string]string{
				"self" + drn.FieldSeparator + "ref": drn.QuickFormat([]string{"class"}, "self", "ref"),
			}),
			expectError: "cyclic DRN reference",
		},
		{
			name:     "Nested and embedded DRN Reference",
			origin:   originKey{globalID: "host1", file: "host1/etc/file.conf"},
			hostInfo: glbConfig.EndpointInfo{EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"class"}, "field1")),
				Namespace: []string{"class"},
				Fields:    []string{"field1"},
				Expanded:  str.DRN(drn.QuickFormat([]string{"class"}, "field1")),
			},
			extConfigPath: "class",
			extConfig: drnconfig.MapToCfgNode(map[string]string{
				"field1": "prefix-" + drn.QuickFormat([]string{"class"}, "field2"),
				"field2": "value1",
			}),
			expect: "prefix-value1",
		},
		{
			name:     "Nested and embedded DRN Reference 2",
			origin:   originKey{globalID: "host1", file: "host1/etc/file.conf"},
			hostInfo: glbConfig.EndpointInfo{EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"class"}, "field1")),
				Namespace: []string{"class"},
				Fields:    []string{"field1"},
				Expanded:  str.DRN(drn.QuickFormat([]string{"class"}, "field1")),
			},
			extConfigPath: "class",
			extConfig: drnconfig.MapToCfgNode(map[string]string{
				"field1": drn.QuickFormat([]string{"class"}, "field2") + "suffix",
				"field2": "value1",
			}),
			expect: "value1suffix",
		},
		{
			name:     "Nested and embedded DRN Reference 3",
			origin:   originKey{globalID: "host1", file: "host1/etc/file.conf"},
			hostInfo: glbConfig.EndpointInfo{EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"class"}, "field1")),
				Namespace: []string{"class"},
				Fields:    []string{"field1"},
				Expanded:  str.DRN(drn.QuickFormat([]string{"class"}, "field1")),
			},
			extConfigPath: "class",
			extConfig: drnconfig.MapToCfgNode(map[string]string{
				"field1": "prefix_" + drn.QuickFormat([]string{"class"}, "field2") + "suffix",
				"field2": "value1",
			}),
			expect: "prefix_value1suffix",
		},
		{
			name:     "Two DRNs in one DRN value",
			origin:   originKey{globalID: "host1", file: "host1/etc/file.conf"},
			hostInfo: glbConfig.EndpointInfo{EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"class"}, "field1")),
				Namespace: []string{"class"},
				Fields:    []string{"field1"},
				Expanded:  str.DRN(drn.QuickFormat([]string{"class"}, "field1")),
			},
			extConfigPath: "class",
			extConfig: drnconfig.MapToCfgNode(map[string]string{
				"field1": drn.QuickFormat([]string{"class"}, "prefix") +
					"." +
					drn.QuickFormat([]string{"class"}, "suffix"),
				"prefix": "a",
				"suffix": "b",
			}),
			expect: "a.b",
		},
		{
			name:     "Two DRNs in one DRN value with macro",
			origin:   originKey{globalID: "host1", file: "host1/etc/file.conf"},
			hostInfo: glbConfig.EndpointInfo{EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"class"}, "{{HOSTALIAS}}")),
				Namespace: []string{"class"},
				Fields:    []string{"{{HOSTALIAS}}"},
			},
			extConfigPath: "class",
			extConfig: drnconfig.MapToCfgNode(map[string]string{
				"host1": drn.QuickFormat([]string{"class"}, "prefix") +
					"." +
					drn.QuickFormat([]string{"class"}, "suffix"),
				"prefix": "a",
				"suffix": "b",
			}),
			expect: "a.b",
		},
		{
			name:     "External Config Unsupported Object Reference",
			origin:   originKey{globalID: "host1", file: "host1/etc/file1.conf"},
			hostInfo: glbConfig.EndpointInfo{EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"class"}, "host", "type")),
				Namespace: []string{"class"},
				Fields:    []string{"host", "type"},
			},
			extConfigPath:  "class",
			rawFileContent: []byte(`{"host":{"type":{"sub":"t"}}}`),
			expectError:    "field 'type' must be value of type string (got object)",
		},
		{
			name:     "External Config Unsupported String Middle Reference",
			origin:   originKey{globalID: "host1", file: "host1/etc/file1.conf"},
			hostInfo: glbConfig.EndpointInfo{EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"class"}, "host", "type")),
				Namespace: []string{"class"},
				Fields:    []string{"host", "type"},
			},
			extConfigPath:  "class",
			rawFileContent: []byte(`{"host":"t"}`),
			expectError:    "field 'host' at depth 0 is a string, but expected an object",
		},
		{
			name:     "External Config Unsupported JSON Value type",
			origin:   originKey{globalID: "host1", file: "host1/etc/file1.conf"},
			hostInfo: glbConfig.EndpointInfo{EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"class"}, "host", "type")),
				Namespace: []string{"class"},
				Fields:    []string{"host", "type"},
			},
			extConfigPath:  "class",
			rawFileContent: []byte(`{"host":{"type":4}}`),
			expectError:    "value must be string or object",
		},
		{
			name:     "External Config Empty JSON Object",
			origin:   originKey{globalID: "host1", file: "host1/etc/file1.conf"},
			hostInfo: glbConfig.EndpointInfo{EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"class"}, "host", "type")),
				Namespace: []string{"class"},
				Fields:    []string{"host", "type"},
			},
			extConfigPath:  "class",
			rawFileContent: []byte(`{"host":{}}`),
			expectError:    "empty objects are not permitted",
		},
		{
			name:   "Missing host info",
			origin: originKey{globalID: "host1", file: "host1/etc/file1.conf"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{drn.InternalNamespacePrefix}, "host", "alias")),
				Namespace: []string{drn.InternalNamespacePrefix},
				Fields:    []string{"host", "alias"},
			},
			expectError: "cannot build file macro replacer: missing contextual host info",
		},
		{
			name:     "Max Recursion Depth Hit",
			origin:   originKey{globalID: "host1", file: "host1/etc/file.conf"},
			hostInfo: glbConfig.EndpointInfo{EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"class"}, "a", "b")),
				Namespace: []string{"class"},
				Fields:    []string{"a", "b"},
				Depth:     drn.MaxNesting,
			},
			expectError: "hit maximum recursion",
		},
		{
			name:     "Internal DRN Unrecognized Macro",
			origin:   originKey{globalID: "host1", file: "host1/etc/file.conf"},
			hostInfo: glbConfig.EndpointInfo{EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{drn.InternalNamespacePrefix}, "unknown", "macro")),
				Namespace: []string{drn.InternalNamespacePrefix},
				Fields:    []string{"unknown", "macro"},
			},
			expectError: "invalid DRN: internal DRN not recognized",
		},
		{
			name:     "DRN with unknown macro",
			origin:   originKey{globalID: "host1", file: "host1/etc/file.conf"},
			hostInfo: glbConfig.EndpointInfo{EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"class"}, "{{UNKMACRO}}", "field")),
				Namespace: []string{"class"},
				Fields:    []string{"{{UNKMACRO}}", "field"},
			},
			expectError: "field 1 contains unknown macro",
		},
		{
			name:     "External Config File Not Found",
			origin:   originKey{globalID: "host1", file: "host1/etc/file.conf"},
			hostInfo: glbConfig.EndpointInfo{EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"missing"}, "a", "b")),
				Namespace: []string{"missing"},
				Fields:    []string{"a", "b"},
			},
			extConfigPath: "",
			expectError:   "config load",
		},
		{
			name:     "External Config File Empty",
			origin:   originKey{globalID: "host1", file: "host1/etc/file.conf"},
			hostInfo: glbConfig.EndpointInfo{EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"empty"}, "a", "b")),
				Namespace: []string{"empty"},
				Fields:    []string{"a", "b"},
			},
			extConfigPath:  "empty",
			rawFileContent: []byte(""),
			expectError:    " is referenced but empty",
		},
		{
			name:     "External Config Invalid JSON",
			origin:   originKey{globalID: "host1", file: "host1/etc/file.conf"},
			hostInfo: glbConfig.EndpointInfo{EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			topDRNConfig: &drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"badjson"}, "a", "b")),
				Namespace: []string{"badjson"},
				Fields:    []string{"a", "b"},
			},
			extConfigPath:  "badjson",
			rawFileContent: []byte("{ invalid json }"),
			expectError:    "config parse",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			cfg := glbConfig.Config{
				RepositoryPath: tmpDir,
				HostInfo: map[str.RepoRootDir]glbConfig.EndpointInfo{
					"host1": {},
				},
			}
			ctx = context.WithValue(ctx, global.ConfKey, cfg)

			// Write actual external DRN config to disk
			if test.extConfigPath != "" {
				confDir := filepath.Join(tmpDir, drn.ExternalVariableDirectory)
				err := os.MkdirAll(confDir, 0700)
				if err != nil {
					t.Fatalf("failed creating external config directory: %v", err)
				}
				confPath := filepath.Join(confDir, test.extConfigPath)

				if test.rawFileContent != nil {
					err = os.WriteFile(confPath, test.rawFileContent, 0600)
					if err != nil {
						t.Fatalf("failed to write external config file: %v", err)
					}
				} else {
					err = test.extConfig.WriteConfig(confPath)
					if err != nil {
						t.Fatalf("failed to write external config file: %v", err)
					}
				}
			}

			replacer := NewReplacer(cfg.RepositoryPath, nil)

			gotValue, err := replacer.resolve(ctx, test.origin, test.hostInfo, test.topDRNConfig, nil)
			matches, err := utils.MatchErrorString(err, test.expectError)
			if err != nil {
				t.Fatalf("%v", err)
			} else if matches {
				return
			}

			if test.expect != gotValue {
				t.Fatalf("DRN resolved value mismatch: got: %s expect: %s", gotValue, test.expect)
			}
			if test.topDRNConfig.Resolved != gotValue {
				t.Fatalf("DRN config resolved mismatch: got: %s != DRN val: %s", gotValue, test.topDRNConfig.Resolved)
			}

			// Parent propagation check
			if test.expectParentResolved && test.parentDRC != nil {
				if test.parentDRC.Resolved != gotValue {
					t.Fatalf("parent.resolved not propagated: got %q, expected %q", test.parentDRC.Resolved, gotValue)
				}
			}
		})
	}
}
