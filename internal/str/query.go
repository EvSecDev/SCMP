package str

import "strings"

func Contains[T StringLike](s T, substr string) bool {
	return strings.Contains(string(s), substr)
}

func HasPrefix[T StringLike](s T, prefix T) bool {
	return strings.HasPrefix(string(s), string(prefix))
}

func HasSuffix[T StringLike](s T, suffix T) bool {
	return strings.HasSuffix(string(s), string(suffix))
}

func Count[T StringLike](s T, substr string) int {
	return strings.Count(string(s), substr)
}

func Index[T StringLike](s T, substr string) int {
	return strings.Index(string(s), substr)
}

func EqualFold[T StringLike](a, b T) bool {
	return strings.EqualFold(string(a), string(b))
}
