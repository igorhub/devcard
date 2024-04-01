package file

import (
	"fmt"
	"io"
	"os"
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
