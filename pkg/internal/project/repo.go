package project

import (
	"fmt"
	"os"
	"sync"

	"github.com/igorhub/devcard"
	"github.com/igorhub/devcard/pkg/runtime"
)

type Repo struct {
	Dir          string
	TransientDir string

	DevcardInfo devcard.DevcardInfo

	runLock sync.Mutex
}

func newRepo(project *Project, devcardInfo devcard.DevcardInfo) (*Repo, error) {
	r := &Repo{
		DevcardInfo: devcardInfo,
	}

	var err error
	r.Dir, err = os.MkdirTemp("", "devcards-"+project.Name+"-")
	if err != nil {
		return nil, fmt.Errorf("new repo: %w", err)
	}

	r.TransientDir = runtime.TransientDir(r.Dir)

	return r, nil
}

func (r *Repo) Delete() error {
	if r == nil {
		return nil
	}
	err := os.RemoveAll(r.Dir)
	if err != nil {
		return fmt.Errorf("delete repo: %w", err)
	}
	return nil
}

// Prepare creates files and directories required for building/running the project.
func (r *Repo) Prepare() error {
	os.RemoveAll(r.TransientDir)
	err := os.Mkdir(r.TransientDir, 0700)
	if err != nil {
		return err
	}
	return GenerateMain(r.Dir, r.DevcardInfo)
}
