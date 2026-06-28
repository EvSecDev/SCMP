package parsing

import "strings"

// Applies string replacer across entire string slice
func BulkSliceReplacer(components []string, replacer *strings.Replacer) (new []string) {
	if len(components) == 0 {
		return
	}
	new = make([]string, len(components))
	for index, value := range components {
		new[index] = replacer.Replace(value)
	}
	return
}
