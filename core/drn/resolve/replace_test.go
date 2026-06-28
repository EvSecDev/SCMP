package resolve

import (
	"reflect"
	"scmp/core/deployment"
	"scmp/core/drn"
	"scmp/internal/str"
	"scmp/internal/tests/utils"
	"slices"
	"testing"
)

func TestReplaceHeaderDRNs(t *testing.T) {
	tests := []struct {
		name        string
		state       *Replacer
		hostAlias   str.RepoRootDir
		file        str.LocalRepoPath
		mapping     map[originKey][]*drn.DRC
		header      deployment.FileInfo
		expect      deployment.FileInfo
		expectError string
	}{
		{
			name:      "Basic",
			state:     NewReplacer("/tmp", nil),
			hostAlias: "host1",
			file:      "conf1",
			mapping: map[originKey][]*drn.DRC{
				{
					globalID:    "host1",
					file:        "conf1",
					headerField: headerPreDeploy,
				}: {
					&drn.DRC{
						Original: str.DRNRaw(drn.QuickFormat([]string{"conf1"}, "field1")),
						Resolved: "value1",
					},
				},
			},
			header: deployment.FileInfo{
				Predeploy: []string{"command1", "command2 -" + drn.QuickFormat([]string{"conf1"}, "field1")},
			},
			expect: deployment.FileInfo{
				Predeploy: []string{"command1", "command2 -value1"},
			},
		},
		{
			name:      "Multiple DRNs in same field",
			hostAlias: "host1", file: "conf1",
			mapping: map[originKey][]*drn.DRC{
				{globalID: "host1", file: "conf1", headerField: headerPreDeploy}: {
					{Original: str.DRNRaw(drn.QuickFormat([]string{"a"}, "1")), Resolved: "A"},
					{Original: str.DRNRaw(drn.QuickFormat([]string{"b"}, "2")), Resolved: "B"},
				},
			},
			header: deployment.FileInfo{Predeploy: []string{"cmd " +
				drn.QuickFormat([]string{"a"}, "1") + " " +
				drn.QuickFormat([]string{"b"}, "2")}},
			expect: deployment.FileInfo{Predeploy: []string{"cmd A B"}},
		},
		{
			name:      "DRNs across multiple fields",
			hostAlias: "host1", file: "conf1",
			mapping: map[originKey][]*drn.DRC{
				{globalID: "host1", file: "conf1", headerField: headerPreDeploy}: {
					{Original: str.DRNRaw(drn.QuickFormat([]string{"x"}, "1")), Resolved: "X"},
				},
				{globalID: "host1", file: "conf1", headerField: headerInstall}: {
					{Original: str.DRNRaw(drn.QuickFormat([]string{"y"}, "2")), Resolved: "Y"},
				},
			},
			header: deployment.FileInfo{
				Predeploy: []string{"start " + drn.QuickFormat([]string{"x"}, "1")},
				Install:   []string{"setup " + drn.QuickFormat([]string{"y"}, "2")},
			},
			expect: deployment.FileInfo{
				Predeploy: []string{"start X"},
				Install:   []string{"setup Y"},
			},
		},
		{
			name:      "Multiple occurrences of same DRN in one command",
			hostAlias: "host1", file: "conf1",
			mapping: map[originKey][]*drn.DRC{
				{globalID: "host1", file: "conf1", headerField: headerChecks}: {
					{Original: str.DRNRaw(drn.QuickFormat([]string{"cmd"}, "dep")), Resolved: "resolved-dep"},
				},
			},
			header: deployment.FileInfo{Checks: []string{drn.QuickFormat([]string{"cmd"}, "dep") + " && " + drn.QuickFormat([]string{"cmd"}, "dep")}},
			expect: deployment.FileInfo{Checks: []string{"resolved-dep && resolved-dep"}},
		},
		{
			name:      "Empty original DRN should be skipped (impl needs guard)",
			hostAlias: "host1", file: "conf1",
			mapping: map[originKey][]*drn.DRC{
				{globalID: "host1", file: "conf1", headerField: headerPreDeploy}: {
					{Original: "", Resolved: "BAD"}, // Should be skipped
					{Original: str.DRNRaw(drn.QuickFormat([]string{"k"}, "1")), Resolved: "K"},
				},
			},
			header: deployment.FileInfo{Predeploy: []string{"cmd " + drn.QuickFormat([]string{"k"}, "1")}},
			expect: deployment.FileInfo{Predeploy: []string{"cmd K"}},
		},
		{
			name:      "Nil DRN in mapping slice should be skipped",
			hostAlias: "host1", file: "conf1",
			mapping: map[originKey][]*drn.DRC{
				{globalID: "host1", file: "conf1", headerField: headerPreDeploy}: {nil, {
					Original: str.DRNRaw(drn.QuickFormat([]string{"z"}, "1")), Resolved: "Z"},
				},
			},
			header: deployment.FileInfo{Predeploy: []string{"cmd " + drn.QuickFormat([]string{"z"}, "1")}},
			expect: deployment.FileInfo{Predeploy: []string{"cmd Z"}},
		},
		{
			name:      "Empty header fields are gracefully skipped",
			hostAlias: "host1", file: "conf1",
			mapping: map[originKey][]*drn.DRC{
				{globalID: "host1", file: "conf1", headerField: headerPreDeploy}: {
					{Original: str.DRNRaw(drn.QuickFormat([]string{"a"}, "x")), Resolved: "X"},
				},
			},
			header: deployment.FileInfo{Predeploy: nil, Install: []string{}},
			expect: deployment.FileInfo{Predeploy: nil, Install: []string{}},
		},
		{
			name:      "No DRNs to replace returns header unchanged",
			hostAlias: "host1", file: "conf1",
			mapping: map[originKey][]*drn.DRC{
				{globalID: "host1", file: "conf1", headerField: headerPreDeploy}: {},
			},
			header: deployment.FileInfo{Predeploy: []string{"plain command"}},
			expect: deployment.FileInfo{Predeploy: []string{"plain command"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.state == nil {
				test.state = NewReplacer("/", nil)
			}
			test.state.originOfDRN = test.mapping

			gotHeader, _, err := test.state.ReplaceHeaderDRNs(test.hostAlias, test.file, test.header)
			matches, err := utils.MatchErrorString(err, test.expectError)
			if err != nil {
				t.Fatalf("%v", err)
			} else if matches {
				return
			}

			if !reflect.DeepEqual(gotHeader, test.expect) {
				t.Errorf("got: %#v\n", gotHeader)
				t.Errorf("exp: %#v\n", test.expect)
				t.Errorf("output mismatch")
			}
		})
	}
}

func TestReplaceDRNs(t *testing.T) {
	tests := []struct {
		name        string
		state       *Replacer
		hostAlias   str.RepoRootDir
		file        str.LocalRepoPath
		mapping     map[originKey][]*drn.DRC
		input       []byte
		expect      []byte
		expectError string
	}{
		{
			name:      "Basic",
			state:     NewReplacer("/tmp", nil),
			hostAlias: "host1",
			file:      "conf1",
			mapping: map[originKey][]*drn.DRC{
				{
					globalID: "host1",
					file:     "conf1",
				}: {
					&drn.DRC{
						Original: str.DRNRaw(drn.QuickFormat([]string{"conf1"}, "field1")),
						Resolved: "value1",
					},
				},
			},
			input:  []byte("data data data data " + drn.QuickFormat([]string{"conf1"}, "field1") + " more data more data"),
			expect: []byte("data data data data value1 more data more data"),
		},
		{
			name:      "Multiple DRNs in payload",
			hostAlias: "host1", file: "conf1",
			mapping: map[originKey][]*drn.DRC{
				{globalID: "host1", file: "conf1"}: {
					{Original: str.DRNRaw(drn.QuickFormat([]string{"a"}, "a")), Resolved: "A"},
					{Original: str.DRNRaw(drn.QuickFormat([]string{"a"}, "b")), Resolved: "B"},
				},
			},
			input:  []byte(drn.QuickFormat([]string{"a"}, "a") + " middle " + drn.QuickFormat([]string{"a"}, "b")),
			expect: []byte("A middle B"),
		},
		{
			name:      "Multiple occurrences of same DRN",
			hostAlias: "host1", file: "conf1",
			mapping: map[originKey][]*drn.DRC{
				{globalID: "host1", file: "conf1"}: {{Original: str.DRNRaw(drn.QuickFormat([]string{"a"}, "dep")), Resolved: "R"}},
			},
			input: []byte(
				drn.QuickFormat([]string{"a"}, "dep") + " " +
					drn.QuickFormat([]string{"a"}, "dep") + " " +
					drn.QuickFormat([]string{"a"}, "dep"),
			),
			expect: []byte("R R R"),
		},
		{
			name:      "Unresolved DRN (empty resolved) returns error",
			hostAlias: "host1", file: "conf1",
			mapping: map[originKey][]*drn.DRC{
				{globalID: "host1", file: "conf1"}: {{Original: str.DRNRaw(drn.QuickFormat([]string{"a"}, "x")), Resolved: ""}},
			},
			input:       []byte(drn.QuickFormat([]string{"a"}, "x")),
			expectError: "unresolved DRN: " + drn.QuickFormat([]string{"a"}, "x"),
		},
		{
			name:      "Empty input data returns nil",
			hostAlias: "host1", file: "conf1",
			mapping: map[originKey][]*drn.DRC{
				{globalID: "host1", file: "conf1"}: {{Original: str.DRNRaw(drn.QuickFormat([]string{"a"}, "x")), Resolved: "X"}},
			},
			input:  []byte{},
			expect: nil,
		},
		{
			name:      "Empty original DRN skipped gracefully",
			hostAlias: "host1", file: "conf1",
			mapping: map[originKey][]*drn.DRC{
				{globalID: "host1", file: "conf1"}: {
					{Original: "", Resolved: "BAD"},
					{Original: str.DRNRaw(drn.QuickFormat([]string{"a"}, "k")), Resolved: "K"},
				},
			},
			input:  []byte(drn.QuickFormat([]string{"a"}, "k")),
			expect: []byte("K"),
		},
		{
			name:      "No DRNs to replace returns exact copy",
			hostAlias: "host1", file: "conf1",
			mapping: map[originKey][]*drn.DRC{
				{globalID: "host1", file: "conf1"}: {},
			},
			input:  []byte("plain data"),
			expect: []byte("plain data"),
		},
		{
			name:      "DRN at exact start and end of payload",
			hostAlias: "host1", file: "conf1",
			mapping: map[originKey][]*drn.DRC{
				{globalID: "host1", file: "conf1"}: {
					{Original: str.DRNRaw(drn.QuickFormat([]string{"a"}, "start")), Resolved: "S"},
					{Original: str.DRNRaw(drn.QuickFormat([]string{"a"}, "end")), Resolved: "E"},
				},
			},
			input:  []byte(drn.QuickFormat([]string{"a"}, "start") + " " + drn.QuickFormat([]string{"a"}, "end")),
			expect: []byte("S E"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.state == nil {
				test.state = NewReplacer("/", nil)
			}
			test.state.originOfDRN = test.mapping

			gotData, _, err := test.state.ReplaceDRNs(test.hostAlias, test.file, test.input)
			matches, err := utils.MatchErrorString(err, test.expectError)
			if err != nil {
				t.Fatalf("%v", err)
			} else if matches {
				return
			}

			if !slices.Equal(gotData, test.expect) {
				t.Errorf("got: '%s'\n", gotData)
				t.Errorf("exp: '%s'\n", test.expect)
				t.Errorf("output mismatch")
			}
		})
	}
}
