// controller
package main

import (
	"sort"
	"testing"
)

func TestFilterHostsAndFiles(t *testing.T) {
	// Lower verbosity for standard prints
	globalVerbosityLevel = 0

	// Mock global vars
	config = Config{
		osPathSeparator: "/",
		hostInfo: map[string]EndpointInfo{
			"host1": {
				deploymentState: "online",
				ignoreUniversal: false,
				universalGroups: map[string]struct{}{"UniversalConfs_Service1": {}, "UniversalConfs": {}},

				endpointName: "host1",
			},
			"host2": {
				deploymentState: "",
				ignoreUniversal: false,
				universalGroups: map[string]struct{}{"UniversalConfs_Service2": {}, "UniversalConfs": {}},
				deploymentFiles: []string{""},
				endpointName:    "host2",
			},
			"host3": {
				deploymentState: "go",
				ignoreUniversal: true,
				universalGroups: map[string]struct{}{"": {}},
				deploymentFiles: []string{""},
				endpointName:    "host3",
			},
			"host4": {
				deploymentState: "",
				ignoreUniversal: false,
				universalGroups: map[string]struct{}{"UniversalConfs": {}},
				deploymentFiles: []string{""},
				endpointName:    "host4",
			},
			"host5": {
				deploymentState: "offline",
				ignoreUniversal: false,
				universalGroups: map[string]struct{}{"UniversalConfs": {}},
				deploymentFiles: []string{""},
				endpointName:    "host5",
			},
		},
		universalDirectory:    "UniversalConfs",
		allUniversalGroups:    map[string][]string{"UniversalConfs_Service1": {"host"}},
		ignoreDeploymentState: false,
	}

	// Test cases
	type TestCase struct {
		name                 string
		commitFiles          map[string]string
		deniedUniversalFiles map[string]map[string]struct{}
		hostOverride         string
		expectedHosts        []string
		expectedFiles        map[string]string
		expectedFilesByHost  map[string][]string
	}
	testCases := []TestCase{
		{
			name: "Standard Deployment Only Host Files",
			commitFiles: map[string]string{
				"host1/etc/resolv.conf":      "create",
				"host1/etc/hosts":            "create",
				"host2/etc/nginx/nginx.conf": "create",
			},
			expectedHosts: []string{"host1", "host2"},
			expectedFiles: map[string]string{
				"host1/etc/resolv.conf":      "create",
				"host1/etc/hosts":            "create",
				"host2/etc/nginx/nginx.conf": "create",
			},
			expectedFilesByHost: map[string][]string{
				"host1": {"host1/etc/resolv.conf", "host1/etc/hosts"},
				"host2": {"host2/etc/nginx/nginx.conf"},
			},
		},
		{
			name: "Host Override Single Host",
			commitFiles: map[string]string{
				"host1/etc/hosts":              "create",
				"host2/etc/network/interfaces": "create",
				"host3/etc/rsyslog.conf":       "create",
			},
			deniedUniversalFiles: map[string]map[string]struct{}{
				"host1": {
					"UniversalConfs/etc/some-file": {},
				},
			},
			hostOverride:  "host3",
			expectedHosts: []string{"host3"},
			expectedFiles: map[string]string{
				"host3/etc/rsyslog.conf": "create",
			},
			expectedFilesByHost: map[string][]string{
				"host3": {"host3/etc/rsyslog.conf"},
			},
		},
		{
			name: "Host Ignores Universal",
			commitFiles: map[string]string{
				"UniversalConfs/etc/resolv.conf": "create",
				"host3/etc/hosts":                "create",
				"host3/etc/crontab":              "create",
			},
			deniedUniversalFiles: map[string]map[string]struct{}{
				"host3": {
					"UniversalConfs/etc/hosts": {},
				},
			},
			hostOverride:  "",
			expectedHosts: []string{"host1", "host2", "host3", "host4"},
			expectedFiles: map[string]string{
				"UniversalConfs/etc/resolv.conf": "create",
				"host3/etc/hosts":                "create",
				"host3/etc/crontab":              "create",
			},
			expectedFilesByHost: map[string][]string{
				"host1": {"UniversalConfs/etc/resolv.conf"},
				"host2": {"UniversalConfs/etc/resolv.conf"},
				"host3": {"host3/etc/hosts", "host3/etc/crontab"},
				"host4": {"UniversalConfs/etc/resolv.conf"},
			},
		},
		{
			name:          "No Commit Files",
			commitFiles:   map[string]string{},
			expectedHosts: []string{},
			expectedFiles: map[string]string{},
			expectedFilesByHost: map[string][]string{
				"": {""},
			},
		},
		{
			name: "Commit Files in Root of Repo",
			commitFiles: map[string]string{
				".example-file":   "create",
				"host3/etc/fstab": "create",
			},
			deniedUniversalFiles: map[string]map[string]struct{}{},
			hostOverride:         "",
			expectedHosts:        []string{"host3"},
			expectedFiles: map[string]string{
				"host3/etc/fstab": "create",
			},
			expectedFilesByHost: map[string][]string{
				"host3": {"host3/etc/fstab"},
			},
		},
		{
			name: "Same File Between Universal And Host",
			commitFiles: map[string]string{
				"UniversalConfs/etc/issue": "create",
				"host2/etc/issue":          "create",
			},
			deniedUniversalFiles: map[string]map[string]struct{}{
				"host2": {
					"UniversalConfs/etc/issue": {},
				},
			},
			expectedHosts: []string{"host1", "host2", "host4"},
			expectedFiles: map[string]string{
				"UniversalConfs/etc/issue": "create",
				"host2/etc/issue":          "create",
			},
			expectedFilesByHost: map[string][]string{
				"host1": {"UniversalConfs/etc/issue"},
				"host2": {"host2/etc/issue"},
				"host4": {"UniversalConfs/etc/issue"},
			},
		},
	}

	// Loop over each test case
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			// Call the function under test
			allDeploymentHosts, allDeploymentFiles := filterHostsAndFiles(test.deniedUniversalFiles, test.commitFiles, test.hostOverride)

			// Validate the hosts
			if len(allDeploymentHosts) != len(test.expectedHosts) {
				t.Errorf("Expected %v hosts, got %v", test.expectedHosts, allDeploymentHosts)
			}
			if !compareArrays(test.expectedHosts, allDeploymentHosts) {
				t.Errorf("Expected deployment hosts %v, but got %v", test.expectedHosts, allDeploymentHosts)
			}

			// Validate the files
			for file, action := range test.expectedFiles {
				_, expectedFileExistsInOutput := allDeploymentFiles[file]
				if !expectedFileExistsInOutput {
					t.Errorf("Expected file '%s', but got nothing", file)
				}
				outputFileAction := allDeploymentFiles[file]
				if outputFileAction != action {
					t.Errorf("Expected action '%s' for file '%s', but got action '%s'", action, file, outputFileAction)
				}
			}

			// Validate files per host
			for _, endpointName := range allDeploymentHosts {
				expectedDeploymentFiles := test.expectedFilesByHost[endpointName]
				deploymentFiles := config.hostInfo[endpointName].deploymentFiles

				if !compareArrays(expectedDeploymentFiles, deploymentFiles) {
					t.Errorf("Host %s: expected files %v, but got %v", endpointName, expectedDeploymentFiles, deploymentFiles)
				}
			}
		})
	}
}

func compareArrays(array1, array2 []string) (arraysIdentical bool) {
	arraysIdentical = false

	// Quick check on length
	if len(array1) != len(array2) {
		return
	}

	// Sort both arrays
	sort.Strings(array1)
	sort.Strings(array2)

	// Compare sorted arrays element by element
	for i := range array1 {
		if array1[i] != array2[i] {
			return
		}
	}

	// They are the same
	arraysIdentical = true
	return
}

func TestHandleFileDependencies(t *testing.T) {
	// Define the test cases directly as literals inside the test function
	testCases := []struct {
		name                string
		hostDeploymentFiles []string
		allFileInfo         map[string]FileInfo
		expected            []string
		expectErr           bool
		expectedNoOutput    bool
	}{
		{
			name:                "Correct lexicography order",
			hostDeploymentFiles: []string{"aaaa", "452dddd", "043cccc", "001bbbb", "010ffff", "002eeee"},
			allFileInfo: map[string]FileInfo{
				"010ffff": {
					dependencies: []string{"043cccc", "452dddd"},
				},
				"aaaa": {
					dependencies: []string{"010ffff"},
				},
				"452dddd": {
					dependencies: []string{},
				},
				"001bbbb": {
					dependencies: []string{},
				},
				"002eeee": {
					dependencies: []string{},
				},
				"043cccc": {
					dependencies: []string{},
				},
			},
			expected:  []string{"001bbbb", "002eeee", "043cccc", "452dddd", "010ffff", "aaaa"},
			expectErr: false,
		},
		{
			name:                "Correct lexicography order different input order",
			hostDeploymentFiles: []string{"043cccc", "aaaa", "010ffff", "001bbbb", "002eeee", "452dddd"},
			allFileInfo: map[string]FileInfo{
				"aaaa": {
					dependencies: []string{"010ffff"},
				},
				"452dddd": {
					dependencies: []string{},
				},
				"043cccc": {
					dependencies: []string{},
				},
				"001bbbb": {
					dependencies: []string{},
				},
				"002eeee": {
					dependencies: []string{},
				},
				"010ffff": {
					dependencies: []string{"043cccc", "452dddd"},
				},
			},
			expected:  []string{"001bbbb", "002eeee", "043cccc", "452dddd", "010ffff", "aaaa"},
			expectErr: false,
		},
		{
			name:                "Valid dependency order",
			hostDeploymentFiles: []string{"file1", "file2", "file3", "file4", "file5"},
			allFileInfo: map[string]FileInfo{
				"file1": {
					dependencies: []string{"file2", "file3"},
				},
				"file2": {
					dependencies: []string{"file3"},
				},
				"file5": {
					dependencies: []string{},
				},
				"file4": {
					dependencies: []string{"file1"},
				},
				"file3": {
					dependencies: []string{},
				},
			},
			expected:  []string{"file3", "file5", "file2", "file1", "file4"},
			expectErr: false,
		},
		{
			name:                "Valid dependency order different input order",
			hostDeploymentFiles: []string{"file2", "file5", "file4", "file3", "file1"},
			allFileInfo: map[string]FileInfo{
				"file1": {
					dependencies: []string{"file2", "file3"},
				},
				"file2": {
					dependencies: []string{"file3"},
				},
				"file5": {
					dependencies: []string{},
				},
				"file4": {
					dependencies: []string{"file1"},
				},
				"file3": {
					dependencies: []string{},
				},
			},
			expected:  []string{"file3", "file5", "file2", "file1", "file4"},
			expectErr: false,
		},
		{
			name:                "Valid dependency order Real Paths",
			hostDeploymentFiles: []string{"/etc/hosts", "/etc/apt/sources.list", "/etc/rsyslog.conf", "/etc/nginx/nginx.conf", "/etc/resolv.conf", "/etc/network/interfaces", "/etc/apt/apt.conf.d/00aptproxy"},
			allFileInfo: map[string]FileInfo{
				"/etc/nginx/nginx.conf": {
					dependencies: []string{"/etc/apt/sources.list"},
				},
				"/etc/apt/sources.list": {
					dependencies: []string{"/etc/apt/apt.conf.d/00aptproxy", "/etc/network/interfaces", "/etc/resolv.conf"},
				},
				"/etc/network/interfaces": {
					dependencies: []string{},
				},
				"/etc/hosts": {
					dependencies: []string{},
				},
				"/etc/rsyslog.conf": {
					dependencies: []string{"/etc/apt/sources.list"},
				},
				"/etc/resolv.conf": {
					dependencies: []string{"/etc/network/interfaces"},
				},
				"/etc/apt/apt.conf.d/00aptproxy": {
					dependencies: []string{"/etc/network/interfaces"},
				},
			},
			expected:  []string{"/etc/hosts", "/etc/network/interfaces", "/etc/apt/apt.conf.d/00aptproxy", "/etc/resolv.conf", "/etc/apt/sources.list", "/etc/nginx/nginx.conf", "/etc/rsyslog.conf"},
			expectErr: false,
		},
		{
			name:                "Non-Present Dependencies",
			hostDeploymentFiles: []string{"/etc/rsyslog.conf", "/etc/nginx/nginx.conf", "/etc/apt/sources.list"},
			allFileInfo: map[string]FileInfo{
				"/etc/nginx/nginx.conf": {
					dependencies: []string{"/etc/apt/sources.list"},
				},
				"/etc/apt/sources.list": {
					dependencies: []string{"/etc/apt/apt.conf.d/00aptproxy", "/etc/network/interfaces", "/etc/resolv.conf"},
				},
				"/etc/rsyslog.conf": {
					dependencies: []string{"/etc/apt/sources.list"},
				},
			},
			expected:  []string{"/etc/apt/sources.list", "/etc/nginx/nginx.conf", "/etc/rsyslog.conf"},
			expectErr: false,
		},
		{
			name:                "Circular dependency",
			hostDeploymentFiles: []string{"file1", "file2", "file3", "file4"},
			allFileInfo: map[string]FileInfo{
				"file1": {
					dependencies: []string{"file2"},
				},
				"file2": {
					dependencies: []string{"file4", "file3"},
				},
				"file3": {
					dependencies: []string{"file1"},
				},
				"file4": {
					dependencies: []string{},
				},
			},
			expected:         nil,
			expectErr:        true,
			expectedNoOutput: true,
		},
		{
			name:                "Circular dependency larger loop",
			hostDeploymentFiles: []string{"file4", "file2", "file1", "file3", "file5", "file6"},
			allFileInfo: map[string]FileInfo{
				"file1": {
					dependencies: []string{"file2"},
				},
				"file2": {
					dependencies: []string{"file3"},
				},
				"file3": {
					dependencies: []string{"file4"},
				},
				"file4": {
					dependencies: []string{"file5"},
				},
				"file5": {
					dependencies: []string{"file6"},
				},
				"file6": {
					dependencies: []string{"file1"},
				},
			},
			expected:         nil,
			expectErr:        true,
			expectedNoOutput: true,
		},
		{
			name:                "No dependencies",
			hostDeploymentFiles: []string{"file3", "file2", "file1"},
			allFileInfo: map[string]FileInfo{
				"file1": {
					dependencies: []string{},
				},
				"file2": {
					dependencies: []string{},
				},
				"file3": {
					dependencies: []string{},
				},
			},
			expected:  []string{"file1", "file2", "file3"},
			expectErr: false,
		},
		{
			name:                "Single file with dependencies",
			hostDeploymentFiles: []string{"file1", "file2"},
			allFileInfo: map[string]FileInfo{
				"file1": {
					dependencies: []string{"file2"},
				},
				"file2": {
					dependencies: []string{},
				},
			},
			expected:  []string{"file2", "file1"},
			expectErr: false,
		},
		{
			name:                "No input",
			hostDeploymentFiles: []string{},
			allFileInfo:         map[string]FileInfo{},
			expected:            []string{},
			expectErr:           false,
			expectedNoOutput:    false,
		},
	}

	// Loop over all test cases
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			result, err := handleFileDependencies(test.hostDeploymentFiles, test.allFileInfo)

			// Check: error, output array, and output validity
			if test.expectedNoOutput && result != nil {
				t.Fatalf("expected no output, got '%v'", result)
			} else if !test.expectedNoOutput && result == nil {
				t.Fatalf("expected output, got nil")
			} else if test.expectErr && err == nil {
				t.Fatalf("expected error, got nil")
			} else if !test.expectErr && err != nil {
				t.Fatalf("expected no error, got '%v'", err)
			} else if !compareArrays(result, test.expected) {
				t.Errorf("expected '%v', got '%v'", test.expected, result)
			}
		})
	}
}
