package project

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/fsnotify/fsnotify"
)

func startWatcher(dir string, events chan projectEvent) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("start watcher in %q: %w", dir, err)
	}

	watchDirs, err := subdirs(dir)
	if err != nil {
		return nil, fmt.Errorf("start watcher: %w", err)
	}
	slices.Sort(watchDirs)
	for _, dir := range watchDirs {
		err = watcher.Add(dir)
		if err != nil {
			watcher.Close()
			return nil, fmt.Errorf("start watcher in %q: %w", dir, err)
		}
	}

	isDir := func(path string) bool {
		if _, ok := slices.BinarySearch(watchDirs, path); ok {
			return true
		}
		meta, err := os.Stat(path)
		if err != nil {
			return false
		}
		return meta.IsDir()
	}

	go func() {
		for {
			select {
			case e, ok := <-watcher.Events:
				if !ok {
					return
				}

				switch e.Op {
				case fsnotify.Create, fsnotify.Write:
					if isDir(e.Name) {
						events <- evRestart{}
						return
					} else {
						events <- evUpdateFile{path: e.Name}
					}
				case fsnotify.Remove, fsnotify.Rename:
					if isDir(e.Name) {
						events <- evRestart{}
						return
					} else {
						events <- evRemoveFile{path: e.Name}
					}
				default:
					// Ignore fsnotify.Chmod, as recommended by fsnotify docs.
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				events <- evFail{err: fmt.Errorf("%s watcher error: %w", dir, err)}
			}
		}
	}()

	return watcher, nil
}

func subdirs(projectDir string) ([]string, error) {
	var result []string
	if !filepath.IsAbs(projectDir) {
		panic(fmt.Errorf("projectDir %q must be an absolute path", projectDir))
	}
	err := filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case !d.IsDir():
			return nil
		case d.Name() == ".git":
			return filepath.SkipDir
		}
		result = append(result, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("building directory structure of %s: %w", projectDir, err)
	}
	return result, nil
}
