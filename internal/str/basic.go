// Wrapper functions around string manipulations for custom string types
package str

import "strings"

func TrimSpace[T StringLike](s T) T {
	return T(strings.TrimSpace(string(s)))
}

func ToLower[T StringLike](s T) T {
	return T(strings.ToLower(string(s)))
}

func ToUpper[T StringLike](s T) T {
	return T(strings.ToUpper(string(s)))
}

func Trim[T StringLike](s T, cutset string) T {
	return T(strings.Trim(string(s), cutset))
}

func TrimPrefix[T StringLike](s T, prefix string) T {
	return T(strings.TrimPrefix(string(s), prefix))
}

func TrimSuffix[T StringLike](s T, suffix string) T {
	return T(strings.TrimSuffix(string(s), suffix))
}

func Replace[T StringLike](s T, old, new string, n int) T {
	return T(strings.Replace(string(s), old, new, n))
}

func ReplaceAll[T StringLike](s T, old, new string) T {
	return T(strings.ReplaceAll(string(s), old, new))
}

func Repeat[T StringLike](s T, count int) T {
	return T(strings.Repeat(string(s), count))
}
