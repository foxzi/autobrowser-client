// Package pathutil has path helpers matching Python's pathlib behaviour.
package pathutil

import "path/filepath"

// Resolve is Path.resolve(): absolute, symlinks followed as far as the path exists.
func Resolve(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	rest := ""
	cur := abs
	for {
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(real, rest)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}
