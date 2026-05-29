package str

func ToStrings[T StringLike](in []T) []string {
	out := make([]string, len(in))

	for i, v := range in {
		out[i] = string(v)
	}

	return out
}

func FromStrings[T StringLike](in []string) []T {
	out := make([]T, len(in))

	for i, v := range in {
		out[i] = T(v)
	}

	return out
}
