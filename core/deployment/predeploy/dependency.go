package predeploy

import (
	"encoding/base64"
	"fmt"
	"scmp/core/deployment"
	"scmp/internal/str"
	"slices"
	"sort"
	"strings"
)

// Correct the order of deployment based on any present dependencies
// Returns independent trees of sorted file lists (each outer array has no dependency on any other outer array)
func HandleFileDependencies(rawDeploymentFiles []str.LocalRepoPath, deployFiles *deployment.HostFiles) (orderedDeploymentFiles [][]str.LocalRepoPath, err error) {
	// Tracking maps
	graph := make(map[str.LocalRepoPath][]str.LocalRepoPath)
	reverseGraph := make(map[str.LocalRepoPath][]str.LocalRepoPath)
	fileSet := make(map[str.LocalRepoPath]bool)

	// Make map of files for this host for easy lookups of file existence
	rawFileSet := make(map[str.LocalRepoPath]struct{})
	for _, file := range rawDeploymentFiles {
		rawFileSet[file] = struct{}{}
	}

	// Create dependency graph
	for _, file := range rawDeploymentFiles {
		info := deployFiles.GetFileInfo(file)
		fileSet[file] = true

		for _, dep := range info.Dependencies {
			// Avoid including dependency file names in deployment that are not a part of this deployment
			_, depInDeployment := rawFileSet[dep]

			if depInDeployment {
				// Forward and reverse dep lookups
				graph[file] = append(graph[file], dep)
				reverseGraph[dep] = append(reverseGraph[dep], file)
				fileSet[dep] = true
			}
		}
	}

	// Find connected trees - undirected DFS
	visited := make(map[str.LocalRepoPath]bool)
	var trees [][]str.LocalRepoPath

	var dfs func(str.LocalRepoPath, *[]str.LocalRepoPath)
	dfs = func(file str.LocalRepoPath, tree *[]str.LocalRepoPath) {
		if visited[file] {
			return
		}

		visited[file] = true
		*tree = append(*tree, file)

		for _, neighbor := range graph[file] {
			dfs(neighbor, tree)
		}
		for _, neighbor := range reverseGraph[file] {
			dfs(neighbor, tree)
		}
	}

	for file := range fileSet {
		if !visited[file] {
			tree := []str.LocalRepoPath{}
			dfs(file, &tree)
			trees = append(trees, tree)
		}
	}

	// Sort each independent tree
	for _, tree := range trees {
		depCount := make(map[str.LocalRepoPath]int)
		subGraph := make(map[str.LocalRepoPath][]str.LocalRepoPath)

		for _, file := range tree {
			for _, dep := range graph[file] {
				if slices.Contains(tree, dep) {
					subGraph[dep] = append(subGraph[dep], file)
					depCount[file]++
				}
			}
		}

		// Start with zero in-degree files
		var queue []str.LocalRepoPath
		for _, file := range tree {
			if depCount[file] == 0 {
				queue = append(queue, file)
			}
		}

		var sorted []str.LocalRepoPath
		for len(queue) > 0 {
			file := queue[0]              // Get lead item in queue
			queue = queue[1:]             // Remove lead item in queue
			sorted = append(sorted, file) // Add lead item to result

			// Add dependents to result when immediately "attached" to parent
			for _, neighbor := range subGraph[file] {
				depCount[neighbor]-- // Decrease dependent count for processed dependent

				// When file has no more parents, add to queue to get added to result
				if depCount[neighbor] == 0 {
					queue = append(queue, neighbor)
				}
			}
		}

		// Return immediately if circular dependency was encountered
		if len(sorted) != len(tree) {
			err = fmt.Errorf("circular dependency detected, unable to continue: offending files: '%v'", tree)
			return
		}

		orderedDeploymentFiles = append(orderedDeploymentFiles, sorted)
	}

	// Create stable ordering of trees based on first file in each
	sort.Slice(orderedDeploymentFiles, func(i, j int) bool {
		return orderedDeploymentFiles[i][0] < orderedDeploymentFiles[j][0]
	})

	return
}

// Handles merging dependency trees when they have overlapping reload commands/reload groups
func MergeDepTrees(depTrees [][]str.LocalRepoPath, deployFiles *deployment.HostFiles) (newDepTrees [][]str.LocalRepoPath) {
	if len(depTrees) == 0 {
		return [][]str.LocalRepoPath{}
	}

	// Tracking maps
	fileToTreeNum := make(map[str.LocalRepoPath]int)
	reloadIDToTreeNum := make(map[str.ReloadID]int)
	reloadGroupToTreeNum := make(map[str.ReloadID]int)

	// Setup file to tree lookups
	for treeNum, tree := range depTrees {
		for _, file := range tree {
			fileToTreeNum[file] = treeNum
		}
	}

	// union-find like structure to merge trees
	parent := make([]int, len(depTrees))
	for treeIndex := range parent {
		parent[treeIndex] = treeIndex
	}
	// Find the root tree index of a given tree
	findRoot := func(treeIndex int) int {
		for parent[treeIndex] != treeIndex {
			// Path compression: point directly to grandparent
			parent[treeIndex] = parent[parent[treeIndex]]
			treeIndex = parent[treeIndex]
		}
		return treeIndex
	}
	// Merge two trees into the same group
	unionTrees := func(treeAIndex, treeBIndex int) {
		rootA, rootB := findRoot(treeAIndex), findRoot(treeBIndex)
		if rootA != rootB {
			parent[rootB] = rootA
		}
	}

	// Identify overlaps between trees
	for file, treeNum := range fileToTreeNum {
		meta := deployFiles.GetFileInfo(file)

		// Reload command overlaps
		if len(meta.Reload) > 0 {
			cmdList := strings.Join(meta.Reload, "|")
			reloadID := str.ReloadID(base64.StdEncoding.EncodeToString([]byte(cmdList)))

			existingTree, reloadIDAlreadyTracked := reloadIDToTreeNum[reloadID]
			if reloadIDAlreadyTracked {
				unionTrees(treeNum, existingTree)
			}
			reloadIDToTreeNum[reloadID] = treeNum
		}

		// Reload group overlaps
		if meta.ReloadGroup != "" {
			if existingTree, ok := reloadGroupToTreeNum[meta.ReloadGroup]; ok {
				unionTrees(treeNum, existingTree)
			}
			reloadGroupToTreeNum[meta.ReloadGroup] = treeNum
		}
	}

	// Merge found overlaps (maintain overall input order)
	merged := make(map[int][]str.LocalRepoPath)
	seen := make(map[int]bool)

	for treeNum, tree := range depTrees {
		root := findRoot(treeNum)
		merged[root] = append(merged[root], tree...)
	}

	for treeNum := range depTrees {
		root := findRoot(treeNum)
		if !seen[root] {
			newDepTrees = append(newDepTrees, merged[root])
			seen[root] = true
		}
	}

	return
}
