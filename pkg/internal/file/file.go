package file

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func Copy(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("copy '%s' to '%s': %w", src, dst, err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("copy '%s' to '%s': %w", src, dst, err)
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	if err != nil {
		return fmt.Errorf("copy '%s' to '%s': %w", src, dst, err)
	}
	return nil
}

func LinkOrCopy(src, dst string) error {
	os.Remove(dst)
	err := os.Link(src, dst)
	if err != nil {
		err = Copy(src, dst)
	}
	return err
}

func ReplaceRootDir(dirFrom, dirTo, path string) string {
	if dirTo == "" {
		panic("dirTo must not be empty")
	}
	if dirTo == dirFrom {
		panic("dirTo must not be the same as dirFrom")
	}
	rel, err := filepath.Rel(dirFrom, path)
	if err != nil {
		panic(fmt.Errorf("path %q must be located in %q", path, dirFrom))
	}
	return filepath.Join(dirTo, rel)
}

func Exists(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	} else if errors.Is(err, os.ErrNotExist) {
		return false
	} else {
		// File may or may not exist. More details in err.
		// We treat the situation as if the file doesn't exist.
		return false
	}
}
