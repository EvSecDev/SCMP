package predeploy

import (
	"scmp/core/deployment"
	"scmp/internal/str"
	"testing"
)

func TestHandleFileDependencies(t *testing.T) {
	testCases := []struct {
		name                string
		hostDeploymentFiles []str.LocalRepoPath
		testFileMeta        map[str.LocalRepoPath]deployment.FileInfo
		expected            [][]str.LocalRepoPath
		expectErr           bool
		expectedNoOutput    bool
	}{
		{
			name:                "Correct lexicography order",
			hostDeploymentFiles: []str.LocalRepoPath{"aaaa", "452dddd", "043cccc", "001bbbb", "010ffff", "002eeee"},
			testFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"010ffff": {
					Dependencies: []str.LocalRepoPath{"043cccc", "452dddd"},
				},
				"aaaa": {
					Dependencies: []str.LocalRepoPath{"010ffff"},
				},
				"452dddd": {
					Dependencies: []str.LocalRepoPath{},
				},
				"001bbbb": {
					Dependencies: []str.LocalRepoPath{},
				},
				"002eeee": {
					Dependencies: []str.LocalRepoPath{},
				},
				"043cccc": {
					Dependencies: []str.LocalRepoPath{},
				},
			},
			expected: [][]str.LocalRepoPath{
				{"001bbbb"},
				{"002eeee"},
				{"043cccc", "452dddd", "010ffff", "aaaa"},
			},
			expectErr: false,
		},
		{
			name:                "Correct lexicography order different input order",
			hostDeploymentFiles: []str.LocalRepoPath{"043cccc", "aaaa", "010ffff", "001bbbb", "002eeee", "452dddd"},
			testFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"aaaa": {
					Dependencies: []str.LocalRepoPath{"010ffff"},
				},
				"452dddd": {
					Dependencies: []str.LocalRepoPath{},
				},
				"043cccc": {
					Dependencies: []str.LocalRepoPath{},
				},
				"001bbbb": {
					Dependencies: []str.LocalRepoPath{},
				},
				"002eeee": {
					Dependencies: []str.LocalRepoPath{},
				},
				"010ffff": {
					Dependencies: []str.LocalRepoPath{"043cccc", "452dddd"},
				},
			},
			expected: [][]str.LocalRepoPath{
				{"001bbbb"},
				{"002eeee"},
				{"043cccc", "452dddd", "010ffff", "aaaa"},
			},
			expectErr: false,
		},
		{
			name:                "Valid dependency order",
			hostDeploymentFiles: []str.LocalRepoPath{"file1", "file2", "file3", "file4", "file5"},
			testFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"file1": {
					Dependencies: []str.LocalRepoPath{"file2", "file3"},
				},
				"file2": {
					Dependencies: []str.LocalRepoPath{"file3"},
				},
				"file5": {
					Dependencies: []str.LocalRepoPath{},
				},
				"file4": {
					Dependencies: []str.LocalRepoPath{"file1"},
				},
				"file3": {
					Dependencies: []str.LocalRepoPath{},
				},
			},
			expected: [][]str.LocalRepoPath{
				{"file3", "file2", "file1", "file4"},
				{"file5"},
			},
			expectErr: false,
		},
		{
			name:                "Valid dependency order different input order",
			hostDeploymentFiles: []str.LocalRepoPath{"file2", "file5", "file4", "file3", "file1"},
			testFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"file1": {
					Dependencies: []str.LocalRepoPath{"file2", "file3"},
				},
				"file2": {
					Dependencies: []str.LocalRepoPath{"file3"},
				},
				"file5": {
					Dependencies: []str.LocalRepoPath{},
				},
				"file4": {
					Dependencies: []str.LocalRepoPath{"file1"},
				},
				"file3": {
					Dependencies: []str.LocalRepoPath{},
				},
			},
			expected: [][]str.LocalRepoPath{
				{"file3", "file2", "file1", "file4"},
				{"file5"},
			},
			expectErr: false,
		},
		{
			name:                "Valid dependency order Real Paths",
			hostDeploymentFiles: []str.LocalRepoPath{"host1/etc/hosts", "host1/etc/apt/sources.list", "host1/etc/rsyslog.conf", "host1/etc/nginx/nginx.conf", "host1/etc/resolv.conf", "host1/etc/network/interfaces", "host1/etc/apt/apt.conf.d/00aptproxy"},
			testFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"host1/etc/nginx/nginx.conf": {
					Dependencies: []str.LocalRepoPath{"host1/etc/apt/sources.list"},
				},
				"host1/etc/apt/sources.list": {
					Dependencies: []str.LocalRepoPath{"host1/etc/apt/apt.conf.d/00aptproxy", "host1/etc/network/interfaces", "host1/etc/resolv.conf"},
				},
				"host1/etc/network/interfaces": {
					Dependencies: []str.LocalRepoPath{},
				},
				"host1/etc/hosts": {
					Dependencies: []str.LocalRepoPath{},
				},
				"host1/etc/rsyslog.conf": {
					Dependencies: []str.LocalRepoPath{"host1/etc/apt/sources.list"},
				},
				"host1/etc/resolv.conf": {
					Dependencies: []str.LocalRepoPath{"host1/etc/network/interfaces"},
				},
				"host1/etc/apt/apt.conf.d/00aptproxy": {
					Dependencies: []str.LocalRepoPath{"host1/etc/network/interfaces"},
				},
			},
			expected: [][]str.LocalRepoPath{
				{"host1/etc/hosts"},
				{"host1/etc/network/interfaces", "host1/etc/apt/apt.conf.d/00aptproxy", "host1/etc/resolv.conf", "host1/etc/apt/sources.list", "host1/etc/nginx/nginx.conf", "host1/etc/rsyslog.conf"},
			},
			expectErr: false,
		},
		{
			name:                "Non-Present Dependencies",
			hostDeploymentFiles: []str.LocalRepoPath{"/etc/rsyslog.conf", "/etc/nginx/nginx.conf", "/etc/apt/sources.list"},
			testFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"/etc/nginx/nginx.conf": {
					Dependencies: []str.LocalRepoPath{"/etc/apt/sources.list"},
				},
				"/etc/apt/sources.list": {
					Dependencies: []str.LocalRepoPath{"/etc/apt/apt.conf.d/00aptproxy", "/etc/network/interfaces", "/etc/resolv.conf"},
				},
				"/etc/rsyslog.conf": {
					Dependencies: []str.LocalRepoPath{"/etc/apt/sources.list"},
				},
			},
			expected: [][]str.LocalRepoPath{
				{"/etc/apt/sources.list", "/etc/nginx/nginx.conf", "/etc/rsyslog.conf"},
			},
			expectErr: false,
		},
		{
			name:                "Circular dependency",
			hostDeploymentFiles: []str.LocalRepoPath{"file1", "file2", "file3", "file4"},
			testFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"file1": {
					Dependencies: []str.LocalRepoPath{"file2"},
				},
				"file2": {
					Dependencies: []str.LocalRepoPath{"file4", "file3"},
				},
				"file3": {
					Dependencies: []str.LocalRepoPath{"file1"},
				},
				"file4": {
					Dependencies: []str.LocalRepoPath{},
				},
			},
			expected:         nil,
			expectErr:        true,
			expectedNoOutput: true,
		},
		{
			name:                "Circular dependency larger loop",
			hostDeploymentFiles: []str.LocalRepoPath{"file4", "file2", "file1", "file3", "file5", "file6"},
			testFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"file1": {
					Dependencies: []str.LocalRepoPath{"file2"},
				},
				"file2": {
					Dependencies: []str.LocalRepoPath{"file3"},
				},
				"file3": {
					Dependencies: []str.LocalRepoPath{"file4"},
				},
				"file4": {
					Dependencies: []str.LocalRepoPath{"file5"},
				},
				"file5": {
					Dependencies: []str.LocalRepoPath{"file6"},
				},
				"file6": {
					Dependencies: []str.LocalRepoPath{"file1"},
				},
			},
			expected:         nil,
			expectErr:        true,
			expectedNoOutput: true,
		},
		{
			name:                "No dependencies",
			hostDeploymentFiles: []str.LocalRepoPath{"file3", "file2", "file1"},
			testFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"file1": {
					Dependencies: []str.LocalRepoPath{},
				},
				"file2": {
					Dependencies: []str.LocalRepoPath{},
				},
				"file3": {
					Dependencies: []str.LocalRepoPath{},
				},
			},
			expected: [][]str.LocalRepoPath{
				{"file1"},
				{"file2"},
				{"file3"},
			},
			expectErr: false,
		},
		{
			name:                "Single file with dependencies",
			hostDeploymentFiles: []str.LocalRepoPath{"file1", "file2"},
			testFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"file1": {
					Dependencies: []str.LocalRepoPath{"file2"},
				},
				"file2": {
					Dependencies: []str.LocalRepoPath{},
				},
			},
			expected: [][]str.LocalRepoPath{
				{"file2", "file1"},
			},
			expectErr: false,
		},
		{
			name:                "No input",
			hostDeploymentFiles: []str.LocalRepoPath{},
			testFileMeta:        map[str.LocalRepoPath]deployment.FileInfo{},
			expected:            [][]str.LocalRepoPath{},
			expectErr:           false,
			expectedNoOutput:    true,
		},
	}

	// Loop over all test cases
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			// Prepare deploy files obj
			deployFiles, err := deployment.NewHostFiles()
			if err != nil {
				t.Fatalf("failed init host files obj: %v", err)
			}
			for path, meta := range test.testFileMeta {
				deployFiles.SetFileMetadata(path, meta)
			}

			result, err := HandleFileDependencies(test.hostDeploymentFiles, deployFiles)

			// Check: error, output array, and output validity
			if test.expectedNoOutput && result != nil {
				t.Fatalf("expected no output, got '%v'", result)
			} else if !test.expectedNoOutput && result == nil {
				t.Fatalf("expected output, got nil")
			} else if test.expectErr && err == nil {
				t.Fatalf("expected error, got nil")
			} else if !test.expectErr && err != nil {
				t.Fatalf("expected no error, got '%v'", err)
			} else if len(test.expected) != len(result) {
				t.Errorf("expected %d independent trees, got %d trees", len(test.expected), len(result))
				t.Errorf("values: expected '%v', got '%v'", test.expected, result)
			} else if len(test.expected) == len(result) {
				for resultTreeIndex, resultTree := range result {
					if !str.CompareArrays(test.expected[resultTreeIndex], resultTree) {
						t.Errorf("expected '%v', got '%v'", test.expected, result)
					}
				}
			}
		})
	}
}

func TestMergeDepTrees(t *testing.T) {
	testCases := []struct {
		name         string
		depTrees     [][]str.LocalRepoPath
		testFileMeta map[str.LocalRepoPath]deployment.FileInfo
		expected     [][]str.LocalRepoPath
	}{
		{
			name: "Fully Separate Files - No changes",
			depTrees: [][]str.LocalRepoPath{
				{"file1"},
				{"file2"},
				{"file3"},
			},
			testFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"file1": {
					Reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					ReloadGroup:    "group1",
					ReloadRequired: true,
				},
				"file2": {
					Reload:         []string{"systemctl restart service2", "systemctl is-active service2"},
					ReloadGroup:    "group2",
					ReloadRequired: true,
				},
				"file3": {
					Reload:         []string{"systemctl restart service3", "systemctl is-active service3"},
					ReloadGroup:    "group3",
					ReloadRequired: true,
				},
			},
			expected: [][]str.LocalRepoPath{
				{"file1"},
				{"file2"},
				{"file3"},
			},
		},
		{
			name: "Files in separate tree with shared group",
			depTrees: [][]str.LocalRepoPath{
				{"file1"},
				{"file2", "file3"},
				{"file4", "file5"},
			},
			testFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"file1": {
					Reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					ReloadGroup:    "group1",
					ReloadRequired: true,
				},
				"file2": {
					Reload:         []string{"systemctl restart service2", "systemctl is-active service2"},
					ReloadGroup:    "group2",
					ReloadRequired: true,
				},
				"file3": {
					Reload:         []string{"systemctl restart service3", "systemctl is-active service3"},
					ReloadGroup:    "sharedgroup",
					ReloadRequired: true,
				},
				"file4": {
					Reload:         []string{"systemctl restart service4", "systemctl is-active service4"},
					ReloadGroup:    "sharedgroup",
					ReloadRequired: true,
				},
				"file5": {
					Reload:         []string{"systemctl restart service5", "systemctl is-active service5"},
					ReloadGroup:    "group5",
					ReloadRequired: true,
				},
			},
			expected: [][]str.LocalRepoPath{
				{"file1"},
				{"file2", "file3", "file4", "file5"},
			},
		},
		{
			name: "Files in separate tree with same reload commands",
			depTrees: [][]str.LocalRepoPath{
				{"file1"},
				{"file2", "file3"},
				{"file4", "file5"},
			},
			testFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"file1": {
					Reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					ReloadGroup:    "group1",
					ReloadRequired: true,
				},
				"file2": {
					Reload:         []string{"systemctl restart service2", "systemctl is-active service2"},
					ReloadGroup:    "group2",
					ReloadRequired: true,
				},
				"file3": {
					Reload:         []string{"systemctl restart sharedservice", "systemctl is-active sharedservice"},
					ReloadGroup:    "group3",
					ReloadRequired: true,
				},
				"file4": {
					Reload:         []string{"systemctl restart sharedservice", "systemctl is-active sharedservice"},
					ReloadGroup:    "group4",
					ReloadRequired: true,
				},
				"file5": {
					Reload:         []string{"systemctl restart service5", "systemctl is-active service5"},
					ReloadGroup:    "group5",
					ReloadRequired: true,
				},
			},
			expected: [][]str.LocalRepoPath{
				{"file1"},
				{"file2", "file3", "file4", "file5"},
			},
		},
		{
			name: "Separate Trees but identical reloads/reload groups",
			depTrees: [][]str.LocalRepoPath{
				{"file1"},
				{"file2"},
				{"file3"},
			},
			testFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"file1": {
					Reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					ReloadGroup:    "group1",
					ReloadRequired: true,
				},
				"file2": {
					Reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					ReloadGroup:    "group1",
					ReloadRequired: true,
				},
				"file3": {
					Reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					ReloadGroup:    "group1",
					ReloadRequired: true,
				},
			},
			expected: [][]str.LocalRepoPath{
				{"file1", "file2", "file3"},
			},
		},
		{
			name: "No Reloads but single group defined for all files in separate trees",
			depTrees: [][]str.LocalRepoPath{
				{"file2", "file3"},
				{"file4"},
				{"file5"},
				{"file6", "file7"},
			},
			testFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"file2": {
					Reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					ReloadRequired: true,
					ReloadGroup:    "Service1",
				},
				"file3": {
					Reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					ReloadRequired: true,
					ReloadGroup:    "Service1",
				},
				"file4": {
					ReloadGroup: "Service1",
				},
				"file5": {
					ReloadGroup: "Service1",
				},
				"file7": {
					Reload:         []string{"service1 checkconf", "service1 reload file7"},
					ReloadRequired: true,
					ReloadGroup:    "Service1",
				},
				"file6": {
					Reload:      []string{"service1 checkconf"},
					ReloadGroup: "Service1",
				},
			},
			expected: [][]str.LocalRepoPath{
				{"file2", "file3", "file4", "file5", "file6", "file7"},
			},
		},
		{
			name: "Commands and Custom Two Different Groups",
			depTrees: [][]str.LocalRepoPath{
				{"file2"},
				{"file3"},
				{"file5", "file4"},
				{"file6"},
			},
			testFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"file3": {
					Reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					ReloadRequired: true,
					ReloadGroup:    "Service1",
				},
				"file2": {
					Reload:         []string{"systemctl restart service1", "systemctl is-active service1"},
					ReloadRequired: true,
					ReloadGroup:    "Service1",
				},
				"file4": {
					ReloadGroup: "Service2",
				},
				"file6": {
					Reload:         []string{"service2 check-conf", "systemctl restart service2", "systemctl is-active service2"},
					ReloadRequired: true,
					ReloadGroup:    "Service2",
				},
				"file5": {
					ReloadGroup: "Service2",
				},
			},
			expected: [][]str.LocalRepoPath{
				{"file2", "file3"},
				{"file5", "file4", "file6"},
			},
		},
		{
			name: "Custom Group No Reloads Pass through",
			depTrees: [][]str.LocalRepoPath{
				{"file3", "file2"},
			},
			testFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"file3": {
					ReloadGroup: "Service1",
				},
				"file2": {
					ReloadGroup: "Service1",
				},
			},
			expected: [][]str.LocalRepoPath{
				{"file3", "file2"},
			},
		},
		{
			name: "No Groups No Reloads",
			depTrees: [][]str.LocalRepoPath{
				{"file1"},
				{"file2"},
				{"file3"},
			},
			testFileMeta: map[str.LocalRepoPath]deployment.FileInfo{
				"file1": {},
				"file2": {},
				"file3": {},
			},
			expected: [][]str.LocalRepoPath{
				{"file1"},
				{"file2"},
				{"file3"},
			},
		},
		{
			name:         "No Input",
			depTrees:     [][]str.LocalRepoPath{},
			testFileMeta: map[str.LocalRepoPath]deployment.FileInfo{},
			expected:     [][]str.LocalRepoPath{},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			// Prepare deploy files obj
			deployFiles, err := deployment.NewHostFiles()
			if err != nil {
				t.Fatalf("failed init host files obj: %v", err)
			}
			for path, meta := range test.testFileMeta {
				deployFiles.SetFileMetadata(path, meta)
			}

			result := MergeDepTrees(test.depTrees, deployFiles)

			if len(test.expected) != len(result) {
				t.Errorf("expected %d independent trees, got %d trees", len(test.expected), len(result))
				t.Errorf("values: expected '%v', got '%v'", test.expected, result)
			} else if len(test.expected) == len(result) {
				for resultTreeIndex, resultTree := range result {
					if !str.CompareArrays(test.expected[resultTreeIndex], resultTree) {
						t.Errorf("expected '%v', got '%v'", test.expected, result)
						return
					}
				}
			}
		})
	}
}
