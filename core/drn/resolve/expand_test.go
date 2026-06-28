package resolve

import (
	"scmp/core/drn"
	"scmp/internal/config"
	"scmp/internal/str"
	"scmp/internal/tests/utils"
	"slices"
	"testing"
)

func TestExpandMacros(t *testing.T) {
	repositoryPath := "/opt/repo"

	tests := []struct {
		name        string
		drnConfig   drn.DRC
		hostInfo    config.EndpointInfo
		filePath    str.LocalRepoPath
		expect      drn.DRC
		expectError string
	}{
		{
			name:      "Empty DRC",
			drnConfig: drn.DRC{},
			hostInfo: config.EndpointInfo{
				EndpointName: "host1",
				Endpoint:     "127.0.0.1:22",
				EndpointUser: "user",
			},
			filePath:    "host1/etc/file1",
			expectError: "characters is below the minimum length of",
		},
		{
			name: "No macros - passthrough",
			drnConfig: drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"host1"}, "file1", "action")),
				Namespace: []string{"host1"},
				Fields:    []string{"file1", "action"},
			},
			hostInfo: config.EndpointInfo{
				EndpointName: "host1",
				Endpoint:     "127.0.0.1:22",
				EndpointUser: "user",
			},
			filePath: "host1/etc/file1",
			expect: drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"host1"}, "file1", "action")),
				Expanded:  str.DRN(drn.QuickFormat([]string{"host1"}, "file1", "action")),
				Namespace: []string{"host1"},
				Fields:    []string{"file1", "action"},
			},
		},
		{
			name: "Field File-based macro",
			drnConfig: drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"host1"}, "{{FILENAME}}", "action")),
				Namespace: []string{"host1"},
				Fields:    []string{"{{FILENAME}}", "action"},
			},
			hostInfo: config.EndpointInfo{
				EndpointName: "host1",
				Endpoint:     "127.0.0.1:22",
				EndpointUser: "user",
			},
			filePath: "host1/etc/file1",
			expect: drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"host1"}, "{{FILENAME}}", "action")),
				Expanded:  str.DRN(drn.QuickFormat([]string{"host1"}, "file1", "action")),
				Namespace: []string{"host1"},
				Fields:    []string{"file1", "action"},
			},
		},
		{
			name: "Field Host-based macro",
			drnConfig: drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"{{HOSTALIAS}}"}, "file1", "action")),
				Namespace: []string{"{{HOSTALIAS}}"},
				Fields:    []string{"file1", "action"},
			},
			hostInfo: config.EndpointInfo{
				EndpointName: "host1",
				Endpoint:     "127.0.0.1:22",
				EndpointUser: "user",
			},
			filePath: "host1/etc/file1",
			expect: drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"{{HOSTALIAS}}"}, "file1", "action")),
				Expanded:  str.DRN(drn.QuickFormat([]string{"host1"}, "file1", "action")),
				Namespace: []string{"host1"},
				Fields:    []string{"file1", "action"},
			},
		},
		{
			name: "File macro with dot character in namespace",
			drnConfig: drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"{{FILENAME}}"}, "file1", "action")),
				Namespace: []string{"{{FILENAME}}"},
				Fields:    []string{"file1", "action"},
			},
			hostInfo: config.EndpointInfo{
				EndpointName: "host1",
				Endpoint:     "127.0.0.1:22",
				EndpointUser: "user",
			},
			filePath: "host1/etc/file1.conf",
			expect: drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"{{FILENAME}}"}, "file1", "action")),
				Expanded:  str.DRN(drn.QuickFormat([]string{"file1.conf"}, "file1", "action")),
				Namespace: []string{"file1.conf"},
				Fields:    []string{"file1", "action"},
			},
		},
		{
			name: "File macro with special character",
			drnConfig: drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"host1"}, "{{FILENAME}}", "action")),
				Namespace: []string{"host1"},
				Fields:    []string{"{{FILENAME}}", "action"},
			},
			hostInfo: config.EndpointInfo{
				EndpointName: "host1",
				Endpoint:     "127.0.0.1:22",
				EndpointUser: "user",
			},
			filePath:    "host1/etc/file1$conf",
			expectError: "contains unsupported characters",
		},
		{
			name: "Unknown field macro",
			drnConfig: drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"{{HOSTALIAS}}"}, "file1", "{{CUSTOM}}")),
				Namespace: []string{"{{HOSTALIAS}}"},
				Fields:    []string{"file1", "{{CUSTOM}}"},
			},
			hostInfo: config.EndpointInfo{
				EndpointName: "host1",
				Endpoint:     "127.0.0.1:22",
				EndpointUser: "user",
			},
			filePath:    "host1/etc/file1",
			expectError: "contains unknown macro",
		},
		{
			name: "Unknown namespace macro",
			drnConfig: drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"{{MYCONF}}"}, "file1", "{{HOSTALIAS}}")),
				Namespace: []string{"{{MYCONF}}"},
				Fields:    []string{"file1", "{{HOSTALIAS}}"},
			},
			hostInfo: config.EndpointInfo{
				EndpointName: "host1",
				Endpoint:     "127.0.0.1:22",
				EndpointUser: "user",
			},
			filePath:    "host1/etc/file1",
			expectError: "contains unknown macro",
		},
		{
			name: "Missing host info",
			drnConfig: drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"{{HOSTALIAS}}"}, "file1", "action")),
				Namespace: []string{"{{HOSTALIAS}}"},
				Fields:    []string{"file1", "action"},
			},
			filePath:    "host1/etc/file1",
			expectError: "cannot build file macro replacer: missing contextual host info",
		},
		{
			name: "Macro resolved repo base dir empty",
			drnConfig: drn.DRC{
				Original:  str.DRNRaw(drn.QuickFormat([]string{"{{REPOBASEDIR}}"}, "file1", "action")),
				Namespace: []string{"{{REPOBASEDIR}}"},
				Fields:    []string{"file1", "action"},
			},
			filePath: "file1",
			hostInfo: config.EndpointInfo{
				EndpointName: "host1",
				Endpoint:     "127.0.0.1:22",
				EndpointUser: "user",
			},
			expectError: "extracted empty value for file macro {{REPOBASEDIR}}",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotDRC, err := ExpandMacros(test.drnConfig, repositoryPath, test.hostInfo, test.filePath)
			matches, err := utils.MatchErrorString(err, test.expectError)
			if err != nil {
				t.Fatalf("%v", err)
			} else if matches {
				return
			}

			if test.expect.Original != gotDRC.Original {
				t.Fatalf("original DRN mismatch: got: %s expect: %s", gotDRC.Original, test.expect.Original)
			}
			if test.expect.Expanded != gotDRC.Expanded {
				t.Fatalf("expanded DRN mismatch: got: %s expect: %s", gotDRC.Expanded, test.expect.Expanded)
			}
			if !slices.Equal(test.expect.Fields, gotDRC.Fields) {
				t.Fatalf("DRN fields mismatch: got: %#v expect: %#v", gotDRC.Fields, test.expect.Fields)
			}
			if !slices.Equal(test.expect.Namespace, gotDRC.Namespace) {
				t.Fatalf("DRN namespace mismatch: got: %#v expect: %#v", gotDRC.Namespace, test.expect.Namespace)
			}
		})
	}
}
