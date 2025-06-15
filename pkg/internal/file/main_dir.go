package file

import (
	"fmt"
	"hash/maphash"
	"path/filepath"
	"slices"

	"github.com/igorhub/devcard"
)

var hashSeed = maphash.MakeSeed()

func DevcardMainDir(meta devcard.DevcardMeta) string {
	// If the devcard is located in a main package, our main function must be
	// placed in the same director.
	if meta.Package == "main" {
		return filepath.Dir(meta.Path)
	}

	dir := fmt.Sprintf("gen_main_%s_%d", filepath.Base(meta.ImportPath), maphash.String(hashSeed, meta.ImportPath))

	// If a devcard path is located in an internal directory, our main
	// package must have the same root.
	parts := splitPath(meta.Path)
	if i := indexLast(parts, "internal"); i > 0 {
		parts = append(parts[:i+1], dir)
		return filepath.Join(parts...)
	}

	// Otherwise, we may create the directory for our main package anywhere;
	// we'll use a new directory in the root of the repo.
	return dir
}

func splitPath(path string) []string {
	var result []string
	for {
		var last string
		path, last = filepath.Dir(path), filepath.Base(path)
		result = append(result, last)
		if last == path {
			break
		}
	}
	slices.Reverse(result)
	return result
}

func indexLast(s []string, v string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == v {
			return i
		}
	}
	return -1
}
