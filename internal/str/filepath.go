package str

import "path/filepath"

func FilePathBase[T StringLike](s T) T {
	return T(filepath.Base(string(s)))
}

func FilePathDir[T StringLike](s T) T {
	return T(filepath.Dir(string(s)))
}

func FilePathJoin[T StringLike](s ...T) T {
	var strArgs []string
	for _, input := range s {
		strArgs = append(strArgs, string(input))
	}
	return T(filepath.Join(strArgs...))
}

func FilePathAbs[T StringLike](s T) (T, error) {
	path, err := filepath.Abs(string(s))
	return T(path), err
}

func FilePathExt[T StringLike](s T) T {
	return T(filepath.Ext(string(s)))
}
