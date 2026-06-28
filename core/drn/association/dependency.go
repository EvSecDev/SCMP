package association

import (
	"fmt"
	"scmp/internal/str"
)

// Computes the transitive reverse-dependency closure for each root DRN.
// DepSet marks all DRNs that are transitively referenced by any root.
// DepToRoot maps each dependent back to the root that pulled it in.
// This is used by classifyFileReferences to know whether finding a DRN in a file means the file relates to the
// original query or just happens to share a dependency (only roots and their transitive dependents count).
func (ref *ReferenceFinder) buildDependentSet(roots []str.DRN) (depSet map[string]bool, depToRoot map[string]string, err error) {
	err = ref.ensureReverseDeps()
	if err != nil {
		err = fmt.Errorf("reverse dependents: %w", err)
		return
	}

	rootSet := make(map[string]struct{}, len(roots))
	seeds := make([]string, len(roots))
	for index, root := range roots {
		rootSet[string(root)] = struct{}{}
		seeds[index] = string(root)
	}

	depSet = make(map[string]bool)
	depToRoot = make(map[string]string)

	visitFunc := func(current, dep string) {
		depSet[dep] = true
		_, isRoot := rootSet[current]
		root, exists := depToRoot[current]
		if isRoot {
			depToRoot[dep] = current
		} else if exists {
			depToRoot[dep] = root
		}
	}

	ref.bfsReverseDeps(seeds, visitFunc)
	return
}

// Performs a BFS over the reverse-dependency graph to collect every DRN (root or transitive) reachable from the given roots.
// This gives us the complete set of search terms needed for the file searcher.
func (ref *ReferenceFinder) traverseReverseDeps(roots []str.DRN) (deps map[string]struct{}) {
	seeds := make([]string, len(roots))
	for i, r := range roots {
		seeds[i] = string(r)
	}
	deps = ref.bfsReverseDeps(seeds, nil)
	return
}

// The shared BFS engine both traverseReverseDeps and buildDependentSet use.
// The visit callback lets buildDependentSet attach root-tracking metadata without duplicating the traversal logic.
// When visit is nil, only the set of reachable DRNs is needed.
func (ref *ReferenceFinder) bfsReverseDeps(seeds []string, visit func(current, dep string)) (deps map[string]struct{}) {
	deps = make(map[string]struct{}, len(seeds))
	queue := make([]string, 0, len(seeds))
	for _, seed := range seeds {
		deps[seed] = struct{}{}
		queue = append(queue, seed)
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, dep := range ref.reverseDeps[current] {
			depStr := string(dep)
			_, ok := deps[depStr]
			if ok {
				continue
			}
			deps[depStr] = struct{}{}
			if visit != nil {
				visit(current, depStr)
			}
			queue = append(queue, depStr)
		}
	}
	return
}
