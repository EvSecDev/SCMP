package sshconfig

import (
	"scmp/internal/config"
	"scmp/internal/str"
	"testing"
)

func TestFilterHostGroups(t *testing.T) {
	// Mock global
	var config config.Config
	config.UniversalDirectory = "UniversalConfs"

	tests := []struct {
		endpointName                 str.RepoRootDir
		universalGroupsCSV           string
		ignoreUniversalString        string
		expectedHostIgnoresUniversal bool
		expectedHostUniversalGroups  map[str.RepoRootDir]struct{}
		expectedAllUniversalGroups   map[str.RepoRootDir][]str.RepoRootDir
	}{
		{
			endpointName:                 "host1",
			universalGroupsCSV:           "group1,group2",
			ignoreUniversalString:        "no",
			expectedHostIgnoresUniversal: false,
			expectedHostUniversalGroups: map[str.RepoRootDir]struct{}{
				"group1":         {},
				"group2":         {},
				"UniversalConfs": {}, // Default universal group should be added
			},
			expectedAllUniversalGroups: map[str.RepoRootDir][]str.RepoRootDir{
				"group1":         {"host1"},
				"group2":         {"host1"},
				"UniversalConfs": {"host1"},
			},
		},
		{
			endpointName:                 "host2",
			universalGroupsCSV:           "group1",
			ignoreUniversalString:        "yes",
			expectedHostIgnoresUniversal: true,
			expectedHostUniversalGroups: map[str.RepoRootDir]struct{}{
				"group1": {},
			},
			expectedAllUniversalGroups: map[str.RepoRootDir][]str.RepoRootDir{
				"group1": {"host2"},
			},
		},
		{
			endpointName:                 "host3",
			universalGroupsCSV:           "",
			ignoreUniversalString:        "no",
			expectedHostIgnoresUniversal: false,
			expectedHostUniversalGroups: map[str.RepoRootDir]struct{}{
				"UniversalConfs": {},
			},
			expectedAllUniversalGroups: map[str.RepoRootDir][]str.RepoRootDir{
				"UniversalConfs": {"host3"},
			},
		},
	}

	for _, test := range tests {
		// Reset global state for each test
		config.AllUniversalGroups = make(map[str.RepoRootDir][]str.RepoRootDir)

		t.Run(string(test.endpointName), func(t *testing.T) {
			// Run the function
			hostIgnoresUniversal, hostUniversalGroups := filterHostGroups(config, test.endpointName, test.universalGroupsCSV, test.ignoreUniversalString)

			// Check if the results match expectations
			if hostIgnoresUniversal != test.expectedHostIgnoresUniversal {
				t.Errorf("expected HostIgnoresUniversal to be %v, got %v", test.expectedHostIgnoresUniversal, hostIgnoresUniversal)
			}

			if len(hostUniversalGroups) != len(test.expectedHostUniversalGroups) {
				t.Errorf("expected HostUniversalGroups length to be %d, got %d", len(test.expectedHostUniversalGroups), len(hostUniversalGroups))
			} else {
				for group := range test.expectedHostUniversalGroups {
					if _, exists := hostUniversalGroups[group]; !exists {
						t.Errorf("expected HostUniversalGroups to contain group %s", group)
					}
				}
			}

			// Check the global map AllUniversalGroups
			for group, expectedHosts := range test.expectedAllUniversalGroups {
				if len(config.AllUniversalGroups[group]) != len(expectedHosts) {
					t.Errorf("expected %d hosts in group %s, got %d", len(expectedHosts), group, len(config.AllUniversalGroups[group]))
				} else {
					for _, expectedHost := range expectedHosts {
						found := false
						for _, host := range config.AllUniversalGroups[group] {
							if host == expectedHost {
								found = true
								break
							}
						}
						if !found {
							t.Errorf("expected host %s in group %s, but it was not found", expectedHost, group)
						}
					}
				}
			}
		})
	}
}
