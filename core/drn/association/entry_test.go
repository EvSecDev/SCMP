package association

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"scmp/core/drn"
	"scmp/core/drn/drnconfig"
	"scmp/internal/config"
	"scmp/internal/str"
	"scmp/internal/tests/utils"
	"slices"
	"testing"
)

func TestReferenceFinder(t *testing.T) {
	SharedUniversalDir := "Universal"

	tests := []struct {
		name            string
		allDRNConfs     map[string]drnconfig.CfgNode
		testFiles       map[string]string
		hosts           map[str.RepoRootDir]config.EndpointInfo
		universalGroups map[str.RepoRootDir][]str.RepoRootDir
		inputDRNs       []string
		expectPaths     []str.LocalRepoPath
		expectHosts     []str.RepoRootDir
		expectError     string
	}{
		{
			name: "No input DRNs",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"cfg1": drnconfig.MapToCfgNode(map[string]string{
					"field1": "value1",
				}),
			},
			testFiles: map[string]string{
				"host1/etc/conf": "primary: " + drn.QuickFormat([]string{"cfg1"}, "field1") + " . some other stuff",
			},
			hosts:       map[str.RepoRootDir]config.EndpointInfo{"host1": {EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"}},
			inputDRNs:   []string{},
			expectPaths: []str.LocalRepoPath{},
			expectHosts: []str.RepoRootDir{},
		},
		{
			name: "Empty input DRN list",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"cfg1": drnconfig.MapToCfgNode(map[string]string{
					"field1": "value1",
				}),
			},
			testFiles: map[string]string{
				"host1/etc/conf": "setting: " + drn.QuickFormat([]string{"cfg1"}, "field1"),
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			},
			inputDRNs:   []string{},
			expectPaths: nil,
			expectHosts: nil,
		},
		{
			name: "Empty hostInfo (all files ignored)",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"cfg1": drnconfig.MapToCfgNode(map[string]string{
					"field1": "value1",
				}),
			},
			testFiles: map[string]string{
				"host1/etc/conf":                 "setting: " + drn.QuickFormat([]string{"cfg1"}, "field1"),
				SharedUniversalDir + "/etc/conf": "other: " + drn.QuickFormat([]string{"cfg1"}, "field1"),
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{},
			inputDRNs: []string{
				drn.QuickFormat([]string{"cfg1"}, "field1"),
			},
			expectPaths: nil,
			expectHosts: nil,
		},
		{
			name: "Single Concrete DRN",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"cfg1": drnconfig.MapToCfgNode(map[string]string{
					"field1": "value1",
				}),
			},
			testFiles: map[string]string{
				"host1/etc/conf": "primary: " + drn.QuickFormat([]string{"cfg1"}, "field1") + " . some other stuff",
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {
					EndpointName: "host1",
					Endpoint:     "127.0.0.1:22",
					EndpointUser: "user",
				},
			},
			inputDRNs:   []string{drn.QuickFormat([]string{"cfg1"}, "field1")},
			expectPaths: []str.LocalRepoPath{"host1/etc/conf"},
			expectHosts: []str.RepoRootDir{"host1"},
		},
		{
			name: "Single Concrete DRN in universal file",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"cfg1": drnconfig.MapToCfgNode(map[string]string{
					"all": "value1",
				}),
			},
			testFiles: map[string]string{
				SharedUniversalDir + "/etc/conf": "primary: " + drn.QuickFormat([]string{"cfg1"}, "all") + " . some other stuff",
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {
					EndpointName: "host1",
					Endpoint:     "127.0.0.1:22",
					EndpointUser: "user",
				},
				"host2": {
					EndpointName: "host2",
					Endpoint:     "127.0.0.2:22",
					EndpointUser: "user",
				},
			},
			inputDRNs:   []string{drn.QuickFormat([]string{"cfg1"}, "all")},
			expectPaths: []str.LocalRepoPath{str.LocalRepoPath(SharedUniversalDir + "/etc/conf")},
			expectHosts: []str.RepoRootDir{"host1", "host2"},
		},
		{
			name: "Single Macro DRN in a universal group file",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"cfg1": drnconfig.MapToCfgNode(map[string]string{
					"host1": "value1",
					"host2": "value2",
				}),
			},
			testFiles: map[string]string{
				"Universal_Apps/etc/conf": "primary: " + drn.QuickFormat([]string{"cfg1"}, "{{HOSTALIAS}}") + " . some other stuff",
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {
					EndpointName: "host1",
					Endpoint:     "127.0.0.1:22",
					EndpointUser: "user",
					UniversalGroups: map[str.RepoRootDir]struct{}{
						"Universal_Apps": {},
					},
				},
				"host2": {
					EndpointName: "host2",
					Endpoint:     "127.0.0.2:22",
					EndpointUser: "user",
					UniversalGroups: map[str.RepoRootDir]struct{}{
						"Universal_Apps": {},
					},
				},
			},
			universalGroups: map[str.RepoRootDir][]str.RepoRootDir{
				"Universal_Apps": {
					"host1",
					"host2",
				},
			},
			inputDRNs:   []string{drn.QuickFormat([]string{"cfg1"}, "host1")}, // Only changed host1 value
			expectPaths: []str.LocalRepoPath{"Universal_Apps/etc/conf"},       // Universal itself is relevant
			expectHosts: []str.RepoRootDir{"host1"},                           // Only host1 is relevant to the change
		},
		{
			name: "Macro DRN in file - concrete DRN in config",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"host1": drnconfig.MapToCfgNode(map[string]string{
					"field1": "value1",
				}),
			},
			testFiles: map[string]string{
				"host1/etc/conf": "primary: " + drn.QuickFormat([]string{"{{HOSTALIAS}}"}, "field1") + " . some other stuff",
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {
					EndpointName: "host1",
					Endpoint:     "127.0.0.1:22",
					EndpointUser: "user",
				},
			},
			inputDRNs:   []string{drn.QuickFormat([]string{"host1"}, "field1")},
			expectPaths: []str.LocalRepoPath{"host1/etc/conf"},
			expectHosts: []str.RepoRootDir{"host1"},
		},
		{
			name: "Macro DRN in file - concrete DRN in value of DRN in config",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"main": drnconfig.MapToCfgNode(map[string]string{
					"shared": "value1",
				}),
				"host1": drnconfig.MapToCfgNode(map[string]string{
					"field1": drn.QuickFormat([]string{"main"}, "shared"),
				}),
			},
			testFiles: map[string]string{
				"host1/etc/conf": "primary: " + drn.QuickFormat([]string{"{{HOSTALIAS}}"}, "field1") + " . some other stuff",
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {
					EndpointName: "host1",
					Endpoint:     "127.0.0.1:22",
					EndpointUser: "user",
				},
			},
			inputDRNs:   []string{drn.QuickFormat([]string{"main"}, "shared")},
			expectPaths: []str.LocalRepoPath{"host1/etc/conf"},
			expectHosts: []str.RepoRootDir{"host1"},
		},
		{
			name: "Concrete DRN in file - macro DRN in value of DRN in config",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"main": drnconfig.MapToCfgNode(map[string]string{
					"host1": "value1",
				}),
				"shared": drnconfig.MapToCfgNode(map[string]string{
					"field1": drn.QuickFormat([]string{"main"}, "{{HOSTALIAS}}"),
				}),
			},
			testFiles: map[string]string{
				"host1/etc/conf": "primary: " + drn.QuickFormat([]string{"shared"}, "field1") + " . some other stuff",
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {
					EndpointName: "host1",
					Endpoint:     "127.0.0.1:22",
					EndpointUser: "user",
				},
			},
			inputDRNs:   []string{drn.QuickFormat([]string{"main"}, "host1")},
			expectPaths: []str.LocalRepoPath{"host1/etc/conf"},
			expectHosts: []str.RepoRootDir{"host1"},
		},
		{
			name: "Universal file with macro DRN matches all applicable hosts",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"host1": drnconfig.MapToCfgNode(map[string]string{
					"field1": "value1",
				}),
				"host2": drnconfig.MapToCfgNode(map[string]string{
					"field1": "value2",
				}),
			},
			testFiles: map[string]string{
				SharedUniversalDir + "/etc/shared.conf": "setting: " + drn.QuickFormat([]string{"{{HOSTALIAS}}"}, "field1"),
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
				"host2": {EndpointName: "host2", Endpoint: "127.0.0.2:22", EndpointUser: "user"},
			},
			inputDRNs:   []string{drn.QuickFormat([]string{"host1"}, "field1")},
			expectPaths: []str.LocalRepoPath{str.LocalRepoPath(SharedUniversalDir + "/etc/shared.conf")},
			expectHosts: []str.RepoRootDir{"host1"},
		},
		{
			name: "Universal group file with file macro",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"Universal_App": drnconfig.MapToCfgNode(map[string]string{
					"field1": "groupval",
				}),
			},
			testFiles: map[string]string{
				"Universal_App/etc/config": "setting: " + drn.QuickFormat([]string{"{{REPOBASEDIR}}"}, "field1"),
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {
					EndpointName:    "host1",
					Endpoint:        "127.0.0.1:22",
					EndpointUser:    "user",
					UniversalGroups: map[str.RepoRootDir]struct{}{"Universal_App": {}},
				},
				"host2": {
					EndpointName: "host2",
					Endpoint:     "127.0.0.2:22",
					EndpointUser: "user",
					// Not in Universal_App
				},
			},
			universalGroups: map[str.RepoRootDir][]str.RepoRootDir{
				"Universal_App": {"host1"},
			},
			inputDRNs:   []string{drn.QuickFormat([]string{"Universal_App"}, "field1")},
			expectPaths: []str.LocalRepoPath{"Universal_App/etc/config"},
			expectHosts: []str.RepoRootDir{"host1"},
		},
		{
			name: "Macro DRN in file does not match unrelated root DRN",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"host1": drnconfig.MapToCfgNode(map[string]string{
					"mainsetting": "value1",
				}),
				"host2": drnconfig.MapToCfgNode(map[string]string{
					"mainsetting": "value2",
				}),
			},
			testFiles: map[string]string{
				"host1/etc/conf": "setting: no drn",
				"host2/etc/conf": "setting: " + drn.QuickFormat([]string{"{{HOSTALIAS}}"}, "mainsetting"),
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
				"host2": {EndpointName: "host2", Endpoint: "127.0.0.2:22", EndpointUser: "user"},
			},
			inputDRNs:   []string{drn.QuickFormat([]string{"host1"}, "mainsetting")},
			expectPaths: nil, // must not match
			expectHosts: nil, // must not match
		},
		{
			name: "Deep dependency chain: file references leaf DRN",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"cfg": drnconfig.MapToCfgNode(map[string]string{
					"a": "leafvalue",
					"b": drn.QuickFormat([]string{"cfg"}, "a"),
					"c": drn.QuickFormat([]string{"cfg"}, "b"),
				}),
			},
			testFiles: map[string]string{
				"host1/etc/conf": "setting: " + drn.QuickFormat([]string{"cfg"}, "c"),
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			},
			inputDRNs:   []string{drn.QuickFormat([]string{"cfg"}, "a")},
			expectPaths: []str.LocalRepoPath{"host1/etc/conf"},
			expectHosts: []str.RepoRootDir{"host1"},
		},
		{
			name: "File contains both macro and concrete DRN - either matching is enough",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"host1": drnconfig.MapToCfgNode(map[string]string{
					"field1": "value1",
				}),
			},
			testFiles: map[string]string{
				"host1/etc/conf": "a: " + drn.QuickFormat([]string{"{{HOSTALIAS}}"}, "field1") + " b: " + drn.QuickFormat([]string{"other"}, "fieldX"),
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			},
			inputDRNs:   []string{drn.QuickFormat([]string{"host1"}, "field1")},
			expectPaths: []str.LocalRepoPath{"host1/etc/conf"},
			expectHosts: []str.RepoRootDir{"host1"},
		},
		{
			name: "Macro expansion matches only one of multiple hosts",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"host1": drnconfig.MapToCfgNode(map[string]string{
					"field1": "value1",
				}),
				// host2 has no such DRN
			},
			testFiles: map[string]string{
				SharedUniversalDir + "/etc/conf": "setting: " + drn.QuickFormat([]string{"{{HOSTALIAS}}"}, "field1"),
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
				"host2": {EndpointName: "host2", Endpoint: "127.0.0.2:22", EndpointUser: "user"},
			},
			inputDRNs:   []string{drn.QuickFormat([]string{"host1"}, "field1")},
			expectPaths: []str.LocalRepoPath{str.LocalRepoPath(SharedUniversalDir + "/etc/conf")},
			expectHosts: []str.RepoRootDir{"host1"},
		},
		{
			name: "All Scenarios",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"host1": drnconfig.MapToCfgNode(map[string]string{
					"service" + drn.FieldSeparator + "path": "/srv",
					"net" + drn.FieldSeparator + "fqdn":     "host1",
				}),
				"host3": drnconfig.MapToCfgNode(map[string]string{
					"field1":   "value1",
					"app-user": "value2",
				}),
				"net": drnconfig.MapToCfgNode(map[string]string{
					"field1": "value1",
					"host1":  "127.0.0.1",
				}),
				"main": drnconfig.MapToCfgNode(map[string]string{
					"field1":        "value1",
					"field2":        "value2",
					"Universal_App": "value3",
				}),
				"shared": drnconfig.MapToCfgNode(map[string]string{
					"field1": drn.QuickFormat([]string{"main"}, "{{HOSTALIAS}}"),
				}),
			},
			testFiles: map[string]string{
				SharedUniversalDir + "/etc/conf": "setting: " + drn.QuickFormat([]string{"{{HOSTALIAS}}"}, "field1"),
				"Universal_App/etc/config":       "setting: " + drn.QuickFormat([]string{"main"}, "{{REPOBASEDIR}}"),
				"host1/etc/hosts":                drn.QuickFormat([]string{"net"}, "{{HOSTALIAS}}") + " " + drn.QuickFormat([]string{"{{HOSTALIAS}}"}, "net", "fqdn"),
				"host3/etc/conf":                 "a: " + drn.QuickFormat([]string{"{{HOSTALIAS}}"}, "field1") + " b: " + drn.QuickFormat([]string{"main"}, "field2"),
				"host3/etc/app":                  "a: " + drn.QuickFormat([]string{"{{HOSTALIAS}}"}, "{{FILENAME}}-{{HOSTLOGINUSER}}"),
				"host2/etc/reverse.conf":         "setting: " + drn.QuickFormat([]string{"shared"}, "field1"),
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user", UniversalGroups: map[str.RepoRootDir]struct{}{"Universal_App": {}}},
				"host2": {EndpointName: "host2", Endpoint: "127.0.0.2:22", EndpointUser: "user"},
				"host3": {EndpointName: "host3", Endpoint: "127.0.0.3:22", EndpointUser: "user", UniversalGroups: map[str.RepoRootDir]struct{}{"Universal_App": {}}},
				"host4": {EndpointName: "host4", Endpoint: "127.0.0.4:22", EndpointUser: "user"},
			},
			inputDRNs: []string{
				drn.QuickFormat([]string{"host1"}, "service", "path"),
				drn.QuickFormat([]string{"host3"}, "field1"),
				drn.QuickFormat([]string{"host3"}, "app-user"),
				drn.QuickFormat([]string{"main"}, "Universal_App"),
				drn.QuickFormat([]string{"main"}, "host2"),
			},
			expectPaths: []str.LocalRepoPath{
				"Universal/etc/conf",
				"Universal_App/etc/config",
				"host2/etc/reverse.conf",
				"host3/etc/app",
				"host3/etc/conf",
			},
			expectHosts: []str.RepoRootDir{"host1", "host2", "host3"},
		},
		{
			name: "Universal macro DRN matches multiple input hosts",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"host1": drnconfig.MapToCfgNode(map[string]string{
					"field1": "value1",
				}),
				"host2": drnconfig.MapToCfgNode(map[string]string{
					"field1": "value2",
				}),
			},
			testFiles: map[string]string{
				SharedUniversalDir + "/etc/conf": "setting: " + drn.QuickFormat([]string{"{{HOSTALIAS}}"}, "field1"),
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
				"host2": {EndpointName: "host2", Endpoint: "127.0.0.2:22", EndpointUser: "user"},
			},
			inputDRNs: []string{
				drn.QuickFormat([]string{"host1"}, "field1"),
				drn.QuickFormat([]string{"host2"}, "field1"),
			},
			expectPaths: []str.LocalRepoPath{str.LocalRepoPath(SharedUniversalDir + "/etc/conf")},
			expectHosts: []str.RepoRootDir{"host1", "host2"},
		},
		{
			name: "Universal file: first macro DRN matches one host, second concrete DRN should add all hosts",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"host1": drnconfig.MapToCfgNode(map[string]string{
					"field1": "value1",
				}),
				"cfgA": drnconfig.MapToCfgNode(map[string]string{
					"settingX": "valueX",
				}),
			},
			testFiles: map[string]string{
				SharedUniversalDir + "/etc/conf": "a: " + drn.QuickFormat([]string{"{{HOSTALIAS}}"}, "field1") +
					" b: " + drn.QuickFormat([]string{"cfgA"}, "settingX"),
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
				"host2": {EndpointName: "host2", Endpoint: "127.0.0.2:22", EndpointUser: "user"},
			},
			inputDRNs: []string{
				drn.QuickFormat([]string{"host1"}, "field1"),
				drn.QuickFormat([]string{"cfgA"}, "settingX"),
			},
			expectPaths: []str.LocalRepoPath{str.LocalRepoPath(SharedUniversalDir + "/etc/conf")},
			expectHosts: []str.RepoRootDir{"host1", "host2"},
		},
		{
			name: "DRN value with file macro",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"cfg1": drnconfig.MapToCfgNode(map[string]string{
					"field1": drn.QuickFormat([]string{"main"}, "{{REPOBASEDIR}}"),
				}),
				"main": drnconfig.MapToCfgNode(map[string]string{
					"Universal":     "global_val",
					"Universal_App": "app_val",
				}),
			},
			testFiles: map[string]string{
				"Universal_App/etc/config": "setting: " + drn.QuickFormat([]string{"cfg1"}, "field1"),
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {
					EndpointName: "host1",
					Endpoint:     "127.0.0.1:22",
					EndpointUser: "user",
					UniversalGroups: map[str.RepoRootDir]struct{}{
						"Universal_App": {},
					},
				},
			},
			universalGroups: map[str.RepoRootDir][]str.RepoRootDir{"Universal_App": {"host1"}},
			inputDRNs:       []string{drn.QuickFormat([]string{"main"}, "Universal_App")},
			expectPaths:     []str.LocalRepoPath{"Universal_App/etc/config"},
			expectHosts:     []str.RepoRootDir{"host1"},
		},
		{
			name: "Multiple Identical DRNs in one file produce the file once",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"cfg1": drnconfig.MapToCfgNode(map[string]string{
					"field1": "value1",
				}),
			},
			testFiles: map[string]string{
				"host1/etc/config": "setting: " + drn.QuickFormat([]string{"cfg1"}, "field1") + " \n " +
					"setting2: " + drn.QuickFormat([]string{"cfg1"}, "field1") + " \n " +
					"setting3: " + drn.QuickFormat([]string{"cfg1"}, "field1"),
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {EndpointName: "host1", Endpoint: "127.0.0.1:22", EndpointUser: "user"},
			},
			inputDRNs:   []string{drn.QuickFormat([]string{"cfg1"}, "field1")},
			expectPaths: []str.LocalRepoPath{"host1/etc/config"},
			expectHosts: []str.RepoRootDir{"host1"},
		},
		{
			name: "Dependency cycle handled A -> B -> A",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"cfg": drnconfig.MapToCfgNode(map[string]string{
					"a": drn.QuickFormat([]string{"cfg"}, "b"),
					"b": drn.QuickFormat([]string{"cfg"}, "a"),
				}),
			},
			testFiles: map[string]string{
				"host1/etc/conf": "setting: " + drn.QuickFormat([]string{"cfg"}, "b"),
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {
					EndpointName: "host1",
					Endpoint:     "127.0.0.1:22",
					EndpointUser: "user",
				},
			},
			inputDRNs: []string{
				drn.QuickFormat([]string{"cfg"}, "a"),
			},
			expectPaths: []str.LocalRepoPath{
				"host1/etc/conf",
			},
			expectHosts: []str.RepoRootDir{
				"host1",
			},
		},
		{
			name: "Diamond dependency graph",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"cfg": drnconfig.MapToCfgNode(map[string]string{
					"a": "leaf",
					"b": drn.QuickFormat([]string{"cfg"}, "a"),
					"c": drn.QuickFormat([]string{"cfg"}, "a"),
					"d": drn.QuickFormat([]string{"cfg"}, "b"),
				}),
			},
			testFiles: map[string]string{
				"host1/etc/conf": "setting: " + drn.QuickFormat([]string{"cfg"}, "d"),
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {
					EndpointName: "host1",
					Endpoint:     "127.0.0.1:22",
					EndpointUser: "user",
				},
			},
			inputDRNs: []string{
				drn.QuickFormat([]string{"cfg"}, "a"),
			},
			expectPaths: []str.LocalRepoPath{
				"host1/etc/conf",
			},
			expectHosts: []str.RepoRootDir{
				"host1",
			},
		},
		{
			name: "Host specific macro does not match other host",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"host2": drnconfig.MapToCfgNode(map[string]string{
					"field1": "value2",
				}),
			},
			testFiles: map[string]string{
				"host2/etc/conf": "setting: " +
					drn.QuickFormat([]string{"{{HOSTALIAS}}"}, "field1"),
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {
					EndpointName: "host1",
					Endpoint:     "127.0.0.1:22",
					EndpointUser: "user",
				},
				"host2": {
					EndpointName: "host2",
					Endpoint:     "127.0.0.2:22",
					EndpointUser: "user",
				},
			},
			inputDRNs: []string{
				drn.QuickFormat([]string{"host1"}, "field1"),
			},
			expectPaths: nil,
			expectHosts: nil,
		},
		{
			name: "Universal group only returns matching hosts",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"host1": drnconfig.MapToCfgNode(map[string]string{
					"field1": "value1",
				}),
				"host2": drnconfig.MapToCfgNode(map[string]string{
					"field1": "value2",
				}),
			},
			testFiles: map[string]string{
				"Universal_App/etc/conf": "setting: " +
					drn.QuickFormat([]string{"{{HOSTALIAS}}"}, "field1"),
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {
					EndpointName: "host1",
					Endpoint:     "127.0.0.1:22",
					EndpointUser: "user",
					UniversalGroups: map[str.RepoRootDir]struct{}{
						"Universal_App": {},
					},
				},
				"host2": {
					EndpointName: "host2",
					Endpoint:     "127.0.0.2:22",
					EndpointUser: "user",
					UniversalGroups: map[str.RepoRootDir]struct{}{
						"Universal_App": {},
					},
				},
				"host3": {
					EndpointName: "host3",
					Endpoint:     "127.0.0.3:22",
					EndpointUser: "user",
				},
			},
			inputDRNs: []string{
				drn.QuickFormat([]string{"host1"}, "field1"),
			},
			expectPaths: []str.LocalRepoPath{
				"Universal_App/etc/conf",
			},
			expectHosts: []str.RepoRootDir{
				"host1",
			},
		},
		{
			name: "Dependency chain through file macro",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"cfg1": drnconfig.MapToCfgNode(map[string]string{
					"field1": drn.QuickFormat([]string{"cfg2"}, "{{REPOBASEDIR}}"),
				}),
				"cfg2": drnconfig.MapToCfgNode(map[string]string{
					"Universal_App": "value1",
				}),
			},
			testFiles: map[string]string{
				"Universal_App/etc/conf": "setting: " +
					drn.QuickFormat([]string{"cfg1"}, "field1"),
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {
					EndpointName: "host1",
					Endpoint:     "127.0.0.1:22",
					EndpointUser: "user",
					UniversalGroups: map[str.RepoRootDir]struct{}{
						"Universal_App": {},
					},
				},
			},
			inputDRNs: []string{
				drn.QuickFormat([]string{"cfg2"}, "Universal_App"),
			},
			expectPaths: []str.LocalRepoPath{
				"Universal_App/etc/conf",
			},
			expectHosts: []str.RepoRootDir{
				"host1",
			},
		},
		{
			name: "Multiple roots converge on same dependency",
			allDRNConfs: map[string]drnconfig.CfgNode{
				"cfg": drnconfig.MapToCfgNode(map[string]string{
					"root1":  "value1",
					"root2":  "value2",
					"shared": drn.QuickFormat([]string{"cfg"}, "root1"),
				}),
			},
			testFiles: map[string]string{
				"host1/etc/conf": "setting: " +
					drn.QuickFormat([]string{"cfg"}, "shared"),
			},
			hosts: map[str.RepoRootDir]config.EndpointInfo{
				"host1": {
					EndpointName: "host1",
					Endpoint:     "127.0.0.1:22",
					EndpointUser: "user",
				},
			},
			inputDRNs: []string{
				drn.QuickFormat([]string{"cfg"}, "root1"),
				drn.QuickFormat([]string{"cfg"}, "root2"),
			},
			expectPaths: []str.LocalRepoPath{
				"host1/etc/conf",
			},
			expectHosts: []str.RepoRootDir{
				"host1",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := config.Config{
				RepositoryPath:     "test",
				HostInfo:           test.hosts,
				UniversalDirectory: str.RepoRootDir(SharedUniversalDir),
				AllUniversalGroups: test.universalGroups,
			}

			// Mocked filesystem
			testReader := func(relPath str.LocalRepoPath) (content []byte, err error) {
				strContent, validFile := test.testFiles[string(relPath)]
				if !validFile {
					err = fmt.Errorf("file not found: %s", relPath)
					return
				}
				content = []byte(strContent)
				return
			}
			testWalker := func() (paths []str.LocalRepoPath, err error) {
				for file := range test.testFiles {
					paths = append(paths, str.LocalRepoPath(file))
				}
				slices.Sort(paths)
				return
			}
			testSearcher := func(ctx context.Context, searchTerms [][]byte) (results map[string]map[string]int, err error) {
				results = make(map[string]map[string]int)
				for path, content := range test.testFiles {
					for _, searchterm := range searchTerms {
						if searchterm == nil {
							err = fmt.Errorf("empty search term supplied")
							return
						}
						if bytes.Contains([]byte(content), searchterm) {
							matches, ok := results[path]
							if !ok {
								matches = make(map[string]int)
							}
							matches[string(searchterm)]++
							results[path] = matches
						}
					}
				}
				return
			}

			allDRNs := make(map[str.LocalRepoPath]map[str.DRN]str.DRNVal)
			for cfgPath, node := range test.allDRNConfs {
				repoPath := filepath.Join(drn.ExternalVariableDirectory, cfgPath)
				drns, err := node.FormatAll(repoPath)
				if err != nil {
					t.Fatalf("drn cfg %s: %v", cfgPath, err)
				}
				allDRNs[str.LocalRepoPath(repoPath)] = drns
			}

			ref, err := NewReferenceFinder(&cfg, allDRNs, testWalker, testSearcher, testReader)
			if err != nil {
				t.Fatalf("unexpected error creating reference obj: %v", err)
			}

			testDRNList := make([]str.DRN, len(test.inputDRNs))
			for index, testDRN := range test.inputDRNs {
				testDRNList[index] = str.DRN(testDRN)
			}

			relatedFiles, relatedHosts, err := ref.FilesReferencingExternals(context.Background(), testDRNList)
			matches, err := utils.MatchErrorString(err, test.expectError)
			if err != nil {
				t.Fatalf("%v", err)
			} else if matches {
				return
			}

			if !slices.Equal(relatedFiles, test.expectPaths) {
				t.Errorf("related files mismatch:\nexpected: %v\ngot:      %v", test.expectPaths, relatedFiles)
			}
			if !slices.Equal(relatedHosts, test.expectHosts) {
				t.Errorf("related hosts mismatch:\nexpected: %v\ngot:      %v", test.expectHosts, relatedHosts)
			}
		})
	}
}
