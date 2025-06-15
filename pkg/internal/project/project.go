package project

import (
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/igorhub/devcard"
	"github.com/igorhub/devcard/pkg/internal/bundle"
	"github.com/igorhub/devcard/pkg/internal/codegenerator"
	"github.com/igorhub/devcard/pkg/internal/config"
	"github.com/igorhub/devcard/pkg/internal/runner"
	"golang.org/x/mod/modfile"
)

type Project struct {
	config.ProjectConfig
	Module string

	cfg        *config.Config
	cardsMeta  DevcardsMetaSlice
	packages   map[string]string
	events     chan projectEvent
	restarts   chan projectEvent
	fatalError error

	fork      *fork
	watcher   *fsnotify.Watcher
	fset      *token.FileSet
	decls     map[string]*printer.CommentedNode
	runners   map[*runner.Runner]struct{}
	generator *codegenerator.Generator
}

func NewProject(cfg *config.Config, projectConfig config.ProjectConfig) *Project {
	p := &Project{
		ProjectConfig: projectConfig,
		cfg:           cfg,
		packages:      make(map[string]string),
		events:        make(chan projectEvent, 256),
		runners:       make(map[*runner.Runner]struct{}),
		generator:     codegenerator.New(projectConfig),
	}

	p.restarts = make(chan projectEvent, 256)
	bundle.Bundle(p.events, p.restarts, 30*time.Millisecond)

	go p.runEventLoop()
	p.Module, p.fatalError = moduleName(p.Dir)
	if p.fatalError == nil {
		p.fork, p.fatalError = newFork(p)
	}

	if p.fatalError == nil {
		p.events <- evRestart{}
	}

	return p
}

func (p *Project) Shutdown() error {
	ch := make(chan error)
	p.events <- evShutdown{err: ch}
	return <-ch
}

func (p *Project) Restart(cfg *config.Config) {
	p.cfg = cfg
	p.events <- evRestart{}
}

func (p *Project) GetDevcards() DevcardsMetaSlice {
	ch := make(chan []devcard.DevcardMeta)
	p.events <- evGetDevcards{result: ch}
	return <-ch
}

func (p *Project) StartRunner(devcardName string) string {
	id := make(chan string)
	p.events <- evStartRunner{devcardName, id}
	return <-id
}

func (p *Project) GetRunner(id string) chan any {
	updates := make(chan chan any)
	p.events <- evGetRunner{id, updates}
	return <-updates
}

func (p *Project) StopRunner(runnerId string) {
	p.events <- evStopRunner{runnerId}
}

func moduleName(projectDir string) (string, error) {
	path := filepath.Join(projectDir, "go.mod")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	mod, err := modfile.Parse(path, data, nil)
	if err != nil {
		return "", err
	}
	return mod.Module.Syntax.Token[1], nil
}
