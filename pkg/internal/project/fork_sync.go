package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/igorhub/devcard"
	"github.com/igorhub/devcard/pkg/internal/file"
)

func (f *fork) syncAll() error {
	err := f.delete()
	if err != nil {
		return fmt.Errorf("syncing %s: %w", f.dir, err)
	}
	os.Mkdir(f.dir, 0700)

	err = f.syncDir()
	if err != nil {
		return fmt.Errorf("syncing %s: %w", f.dir, err)
	}

	err = f.generateInjections()
	if err != nil {
		return fmt.Errorf("syncing %s: %w", f.dir, err)
	}

	err = f.generateMains()
	if err != nil {
		return fmt.Errorf("syncing %s: %w", f.dir, err)
	}

	return nil
}

func lookupDevcardMeta(cards []devcard.DevcardMeta, devcardName string) devcard.DevcardMeta {
	i := slices.IndexFunc(cards, func(meta devcard.DevcardMeta) bool {
		return meta.Name == devcardName
	})
	if i == -1 {
		return devcard.DevcardMeta{}
	}
	return cards[i]
}

func (f *fork) path(pathInProjectDir string) string {
	return file.ReplaceRootDir(f.p.Dir, f.dir, pathInProjectDir)
}

func (f *fork) syncDir() error {
	err := filepath.WalkDir(f.p.Dir, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.Name() == ".git":
			return fs.SkipDir
		case d.IsDir():
			_ = os.Mkdir(f.path(path), 0700)
			return nil
		default:
			return f.syncFile(path, true)
		}
	})
	if err != nil {
		return err
	}
	return nil
}

func (f *fork) syncFile(path string, dontGenerateMain bool) error {
	cards := func() string {
		s := new(strings.Builder)
		for _, meta := range f.p.cardsMeta {
			s.WriteString(meta.Name)
		}
		return s.String()
	}
	cardsBeforeSyncing := cards()
	dst := f.path(path)
	data, err := f.p.updateFile(path)
	if err != nil {
		return fmt.Errorf("sync %s: %w", path, err)
	}
	if data != nil {
		return os.WriteFile(dst, data, 0600)
	}
	err = file.LinkOrCopy(path, dst)
	if err != nil && isDeletedAlready(path) {
		// Some stupid editors create temporary files for no apparent reason and then remove
		// them immediately. In such case, copying might fail, and we must not consider it
		// an error.
		err = nil
	}
	if err != nil {
		return fmt.Errorf("sync %s: %w", path, err)
	}
	if !dontGenerateMain && cardsBeforeSyncing != cards() {
		err = f.generateMains()
	}
	return err
}

func isDeletedAlready(path string) bool {
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	return false
}

func (f *fork) removeFile(path string) error {
	f.p.cardsMeta = slices.DeleteFunc(f.p.cardsMeta, func(meta devcard.DevcardMeta) bool {
		return filepath.Join(f.p.Dir, meta.Path) == path
	})

	_ = os.Remove(f.path(path))
	return nil
}

func (f *fork) generateInjections() error {
	if f.p.Injection == "" {
		return nil
	}
	var errs []error
	for dir, pkg := range f.p.packages {
		path := filepath.Join(f.dir, dir, "generated_devcard_injection.go")
		content := "package " + pkg + "\n\n" + f.p.Injection
		err := os.WriteFile(path, []byte(content), 0664)
		if err != nil {
			errs = append(errs, fmt.Errorf("cannot write code injection into %q", dir))
		}
	}
	return errors.Join(errs...)
}
