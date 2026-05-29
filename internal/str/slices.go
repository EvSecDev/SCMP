package str

import "strings"

func Split[T StringLike](s T, sep string) []T {
	parts := strings.Split(string(s), sep)

	out := make([]T, len(parts))

	for i, p := range parts {
		out[i] = T(p)
	}

	return out
}

func Fields[T StringLike](s T) []T {
	parts := strings.Fields(string(s))

	out := make([]T, len(parts))

	for i, p := range parts {
		out[i] = T(p)
	}

	return out
}

func Lines[T StringLike](s T) []T {
	return Split(s, "\n")
}

func Join[T StringLike](parts []T, sep string) string {
	strs := make([]string, len(parts))

	for i, p := range parts {
		strs[i] = string(p)
	}

	return strings.Join(strs, sep)
}

func JoinAs[T StringLike](parts []T, sep string) T {
	strs := make([]string, len(parts))

	for i, p := range parts {
		strs[i] = string(p)
	}

	return T(strings.Join(strs, sep))
}
