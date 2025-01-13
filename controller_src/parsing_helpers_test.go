// controller
package main

import (
	"fmt"
	"testing"
)

// Unit test for checkForOverride
func TestCheckForOverride(t *testing.T) {
	tests := []struct {
		override     string
		current      string
		expectedSkip bool
	}{
		{"", "host1", false},
		{"host1", "host1", false},
		{"host1,host2", "host1", false},
		{"host1,host2", "host3", true},
		{"host1, host2", "host3", true},
		{"host1, host2, host3, host4, host5, host6", "host3", true},
		{"file1.txt,file2.txt", "file1.txt", false},
		{"file1.txt,file2.txt", "file3.txt", true},
		{"file!@%$^&*(4.txt,file6.txt", "file6.txt", false},
		{"file!@%$^&*(4.txt,file6.txt", "file!@%$^&*(4.txt", false},
	}

	for _, test := range tests {
		testTitle := fmt.Sprintf("Available Items:'%s'-Current Item:'%s'", test.override, test.current)
		t.Run(testTitle, func(t *testing.T) {
			skip := checkForOverride(test.override, test.current)
			if skip != test.expectedSkip {
				t.Errorf("Skip current item? %t; Should skip current item? %t", skip, test.expectedSkip)
			}
		})
	}
}

// Test function for findDeniedUniversalFiles
func TestFindDeniedUniversalFiles(t *testing.T) {
	tests := []struct {
		endpointName        string
		hostFiles           map[string]struct{}
		universalFiles      map[string]struct{}
		universalGroupFiles map[string]map[string]struct{}
		expectedDeniedFiles map[string]struct{}
		universalDirectory  string
		universalGroups     map[string][]string
	}{
		{ // Host has identical file to global universal
			endpointName:   "host1",
			hostFiles:      map[string]struct{}{"etc/fileA": {}, "etc/fileB": {}},
			universalFiles: map[string]struct{}{"etc/fileA": {}, "etc/fileC": {}},
			universalGroupFiles: map[string]map[string]struct{}{
				"UniversalConfs_Group1": {"etc/fileD": {}, "etc/fileE": {}},
			},
			expectedDeniedFiles: map[string]struct{}{
				"UniversalConfs/etc/fileA": {},
			},
			universalDirectory: "UniversalConfs",
			universalGroups: map[string][]string{
				"UniversalConfs_Group1": {"host4", "host7"},
				"UniversalConfs_Group2": {"host10"},
			},
		},
		{ // Host does not have universal file, and is not part of a universal group
			endpointName:   "host2",
			hostFiles:      map[string]struct{}{"etc/fileF": {}, "etc/fileG": {}},
			universalFiles: map[string]struct{}{"etc/fileA": {}, "etc/fileC": {}},
			universalGroupFiles: map[string]map[string]struct{}{
				"UniversalConfs_Group1": {"etc/fileD": {}, "etc/fileG": {}},
			},
			expectedDeniedFiles: map[string]struct{}{},
			universalDirectory:  "UniversalConfs",
			universalGroups: map[string][]string{
				"UniversalConfs_Group1": {"host4", "host7"},
				"UniversalConfs_Group2": {"host10"},
			},
		},
		{ // Host has identical file to global universal
			endpointName:   "host3",
			hostFiles:      map[string]struct{}{"etc/fileB": {}},
			universalFiles: map[string]struct{}{"etc/fileA": {}, "etc/fileB": {}},
			universalGroupFiles: map[string]map[string]struct{}{
				"UniversalConfs_Group1": {"etc/fileD": {}},
			},
			expectedDeniedFiles: map[string]struct{}{
				"UniversalConfs/etc/fileB": {},
			},
			universalDirectory: "UniversalConfs",
			universalGroups: map[string][]string{
				"UniversalConfs_Group1": {"host4", "host7"},
				"UniversalConfs_Group2": {"host3"},
			},
		},
		{ // Host is part of universal group and has identical file
			endpointName:   "host4",
			hostFiles:      map[string]struct{}{"etc/fileB": {}},
			universalFiles: map[string]struct{}{"etc/fileA": {}, "etc/fileC": {}},
			universalGroupFiles: map[string]map[string]struct{}{
				"UniversalConfs_Group1": {"etc/fileB": {}},
			},
			expectedDeniedFiles: map[string]struct{}{
				"UniversalConfs_Group1/etc/fileB": {},
			},
			universalDirectory: "UniversalConfs",
			universalGroups: map[string][]string{
				"UniversalConfs_Group1": {"host4", "host7"},
				"UniversalConfs_Group2": {"host10"},
			},
		},
		{ // Host is in group and has identical file to group and global universal
			endpointName:   "host7",
			hostFiles:      map[string]struct{}{"etc/fileD": {}},
			universalFiles: map[string]struct{}{"etc/fileA": {}, "etc/fileD": {}},
			universalGroupFiles: map[string]map[string]struct{}{
				"UniversalConfs_Group1": {"etc/fileD": {}},
			},
			expectedDeniedFiles: map[string]struct{}{
				"UniversalConfs/etc/fileD":        {},
				"UniversalConfs_Group1/etc/fileD": {},
			},
			universalDirectory: "UniversalConfs",
			universalGroups: map[string][]string{
				"UniversalConfs_Group1": {"host4", "host7"},
				"UniversalConfs_Group2": {"host3"},
			},
		},
		{ // Empty
			endpointName:        "host5",
			hostFiles:           map[string]struct{}{},
			universalFiles:      map[string]struct{}{"etc/fileA": {}, "etc/fileB": {}},
			universalGroupFiles: map[string]map[string]struct{}{},
			expectedDeniedFiles: map[string]struct{}{},
			universalDirectory:  "UniversalConfs",
			universalGroups: map[string][]string{
				"UniversalConfs_Group1": {"host4", "host7"},
				"UniversalConfs_Group2": {"host10"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.endpointName, func(t *testing.T) {
			// Set global required for function
			UniversalDirectory = test.universalDirectory
			UniversalGroups = test.universalGroups

			deniedFiles := findDeniedUniversalFiles(test.endpointName, test.hostFiles, test.universalFiles, test.universalGroupFiles)

			// Check if the denied files match the expected output
			for deniedFile := range test.expectedDeniedFiles {
				if _, exists := deniedFiles[deniedFile]; !exists {
					t.Errorf("Expected denied file %s not found", deniedFile)
				}
			}

			// Ensure there are no extra files in denied files
			for deniedFile := range deniedFiles {
				if _, exists := test.expectedDeniedFiles[deniedFile]; !exists {
					t.Errorf("Unexpected denied file %s found", deniedFile)
				}
			}
		})
	}
}

func TestValidateRepoFile(t *testing.T) {
	// Mock globals for the tests
	OSPathSeparator = "/"
	DeployerEndpoints = []string{"validHost", "validHost2"}
	IgnoreDirectories = []string{"ignoreDir", "ignoreDir2"}
	UniversalDirectory = "UniversalConfs"
	UniversalGroups = map[string][]string{
		"UniversalConfs_Group1": {},
	}

	tests := []struct {
		path     string
		expected struct {
			hostDirName string
			skipFile    bool
		}
	}{
		{"file.txt", struct {
			hostDirName string
			skipFile    bool
		}{"", true}},
		{"ignoreDir/file.txt", struct {
			hostDirName string
			skipFile    bool
		}{"", true}},
		{"validHost/etc/file.txt", struct {
			hostDirName string
			skipFile    bool
		}{"validHost", false}},
		{"UniversalConfs/file.txt", struct {
			hostDirName string
			skipFile    bool
		}{"UniversalConfs", false}},
		{"UniversalConfs_Group1/file.txt", struct {
			hostDirName string
			skipFile    bool
		}{"UniversalConfs_Group1", false}},
		{"invalidDir/file.txt", struct {
			hostDirName string
			skipFile    bool
		}{"", true}},
		{"/etc/file.txt", struct {
			hostDirName string
			skipFile    bool
		}{"", true}},
		{"", struct {
			hostDirName string
			skipFile    bool
		}{"", true}},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			hostDirName, skipFile := validateRepoFile(test.path)
			if skipFile != test.expected.skipFile {
				t.Errorf("expected skipFile to be %t, got %t", test.expected.skipFile, skipFile)
			}
			if !skipFile && hostDirName != test.expected.hostDirName {
				t.Errorf("expected hostDirName to be %s, got %s", test.expected.hostDirName, hostDirName)
			}
		})
	}
}

func TestDetermineFileType(t *testing.T) {
	tests := []struct {
		fileMode string
		expected string
	}{
		{"0100644", "regular"},     // Text file
		{"0120000", "symlink"},     // Special, but able to be handled
		{"0040000", "unsupported"}, // Directory
		{"0160000", "unsupported"}, // Git submodule
		{"0100755", "unsupported"}, // Executable
		{"0100664", "unsupported"}, // Deprecated
		{"0", "unsupported"},       // Empty (no file)
		{"", "unsupported"},        // Empty string
		{"unknown", "unsupported"}, // Unknown - don't process
	}

	for _, test := range tests {
		t.Run(test.fileMode, func(t *testing.T) {
			result := determineFileType(test.fileMode)
			if result != test.expected {
				t.Errorf("determineFileType(%s) = %s; want %s", test.fileMode, result, test.expected)
			}
		})
	}
}

func TestSeparateHostDirFromPath(t *testing.T) {
	tests := []struct {
		localRepoPath    string
		expectedHostDir  string
		expectedFilePath string
	}{
		{"host/dir/file.txt", "host", "/dir/file.txt"},
		{"host2/dir/subdir/file.txt", "host2", "/dir/subdir/file.txt"},
		{"file1.txt", "", ""},
		{"", "", ""},
		{"/home/user/repo/host1/file", "", "/home/user/repo/host1/file"},
		{"!@#$%^&*()_+/etc/file", "!@#$%^&*()_+", "/etc/file"},
	}

	for _, test := range tests {
		t.Run(test.localRepoPath, func(t *testing.T) {
			hostDir, targetFilePath := separateHostDirFromPath(test.localRepoPath)
			if hostDir != test.expectedHostDir {
				t.Errorf("expected hostDir '%s', got '%s'", test.expectedHostDir, hostDir)
			}
			if targetFilePath != test.expectedFilePath {
				t.Errorf("expected targetFilePath '%s', got '%s'", test.expectedFilePath, targetFilePath)
			}
		})
	}
}
