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
		OSPathSeparator: "/",
		HostInfo: map[string]EndpointInfo{
			"host1": {
				DeploymentState: "online",
				IgnoreUniversal: false,
				UniversalGroups: map[string]struct{}{"UniversalConfs_Service1": {}},

				EndpointName: "host1",
			},
			"host2": {
				DeploymentState: "",
				IgnoreUniversal: false,
				UniversalGroups: map[string]struct{}{"UniversalConfs_Service2": {}},
				DeploymentFiles: []string{""},
				EndpointName:    "host2",
			},
			"host3": {
				DeploymentState: "go",
				IgnoreUniversal: true,
				UniversalGroups: map[string]struct{}{"": {}},
				DeploymentFiles: []string{""},
				EndpointName:    "host3",
			},
			"host4": {
				DeploymentState: "",
				IgnoreUniversal: false,
				UniversalGroups: map[string]struct{}{"": {}},
				DeploymentFiles: []string{""},
				EndpointName:    "host4",
			},
			"host5": {
				DeploymentState: "offline",
				IgnoreUniversal: false,
				UniversalGroups: map[string]struct{}{"": {}},
				DeploymentFiles: []string{""},
				EndpointName:    "host5",
			},
		},
		UniversalDirectory: "UniversalConfs",
		AllUniversalGroups: map[string]struct{}{"UniversalConfs_Service1": {}},
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
				deploymentFiles := config.HostInfo[endpointName].DeploymentFiles

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
