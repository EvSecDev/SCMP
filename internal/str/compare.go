package str

import "sort"

func CompareArrays[T StringLike](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}

	aCopy := append([]T(nil), a...)
	bCopy := append([]T(nil), b...)

	sort.Slice(aCopy, func(i, j int) bool {
		return aCopy[i] < aCopy[j]
	})

	sort.Slice(bCopy, func(i, j int) bool {
		return bCopy[i] < bCopy[j]
	})

	for i := range aCopy {
		if aCopy[i] != bCopy[i] {
			return false
		}
	}

	return true
}

func CompareSliceMaps[K comparable, V StringLike](a, b map[K][]V) bool {

	if len(a) != len(b) {
		return false
	}

	for key, aVals := range a {
		bVals, exists := b[key]

		if !exists {
			return false
		}

		if !CompareArrays(aVals, bVals) {
			return false
		}
	}

	return true
}

func CompareMaps[K comparable, V StringLike](a, b map[K]V) bool {

	if len(a) != len(b) {
		return false
	}

	for key, aVal := range a {
		bVal, exists := b[key]

		if !exists || aVal != bVal {
			return false
		}
	}

	return true
}

func EqualMaps[K comparable, S comparable](a, b map[K]map[S]struct{}) bool {
	if len(a) != len(b) {
		return false
	}

	for key, aSet := range a {
		bSet, ok := b[key]

		if !ok {
			return false
		}

		if len(aSet) != len(bSet) {
			return false
		}

		for item := range aSet {
			if _, ok := bSet[item]; !ok {
				return false
			}
		}
	}

	return true
}
