package project

import (
	"fmt"
	"os"
)

type fork struct {
	p   *Project
	dir string
}

func newFork(p *Project) (*fork, error) {
	dir, err := os.MkdirTemp("", "devcards-"+p.Name+"-")
	if err != nil {
		return nil, fmt.Errorf("new fork: %w", err)
	}
	return &fork{p: p, dir: dir}, nil
}

func (r *fork) delete() error {
	err := os.RemoveAll(r.dir)
	if err != nil {
		return fmt.Errorf("delete fork: %w", err)
	}
	return nil
}

func (r *fork) clear() error {
	if err := r.delete(); err != nil {
		return fmt.Errorf("clear fork: %w", err)
	}
	if err := os.Mkdir(r.dir, 0700); err != nil {
		return fmt.Errorf("clear fork: %w", err)
	}
	return nil
}
