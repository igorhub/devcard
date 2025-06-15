package project

import (
	"errors"
	"fmt"
	"go/printer"
	"go/token"
	"log"
	"time"

	"github.com/igorhub/devcard"
	"github.com/igorhub/devcard/pkg/internal/runner"
)

type projectEvent interface {
	act(p *Project) error
}

func (p *Project) runEventLoop() {
	for e := range p.events {
		log.Printf("[ev @%s] %#v\n", p.Name, e)
		err := e.act(p)
		if err != nil {
			p.events <- evFail{err}
			var e *retryError
			if errors.As(err, &e) && e.retryN <= maxRetries {
				go func() {
					time.Sleep(500 * time.Duration(e.retryN) * time.Millisecond)
					p.events <- evRestart{e}
				}()
			}
		}
	}
}

type evUpdateFile struct {
	path string
}

func (e evUpdateFile) act(p *Project) error {
	err := p.fork.syncFile(e.path, false)
	p.generator.AddFile(e.path)
	p.restarts <- evRestartRunners{}
	return err
}

type evRemoveFile struct {
	path string
}

func (e evRemoveFile) act(p *Project) error {
	err := p.fork.removeFile(e.path)
	p.restarts <- evRestartRunners{}
	return err
}

type evRestartRunners struct{}

// MUST return nil
func (e evRestartRunners) act(p *Project) error {
	err := p.fatalError
	if err == nil {
		err = p.generator.Run()
	}
	for r := range p.runners {
		if _, err2 := p.findDevcardMeta(r.DevcardName); err2 != nil {
			err = err2
		}
		r.Restart(p.cfg, err)
	}
	return nil
}

type evGetDevcards struct {
	result chan<- []devcard.DevcardMeta
}

func (e evGetDevcards) act(p *Project) error {
	e.result <- p.cardsMeta
	close(e.result)
	return nil
}

type evGetSource struct {
	decl   string
	source string
	err    error
	result chan<- evGetSource
}

func (e evGetSource) act(p *Project) error {
	// TODO: implement
	return nil
}

type retryError struct {
	retryN int
	err    error
}

const maxRetries = 5

func (e *retryError) Error() string {
	return e.err.Error()
}

func (e *retryError) Unwrap() error {
	return e.err
}

func newRetryError(lastError error, err error) *retryError {
	var last *retryError
	if !errors.As(lastError, &last) {
		last = &retryError{retryN: 0}
	}
	return &retryError{
		retryN: last.retryN + 1,
		err:    err,
	}
}

type evRestart struct {
	lastError error
}

func (e evRestart) act(p *Project) error {
	if p.watcher != nil {
		if err := p.watcher.Close(); err != nil {
			return err
		}
	}

	if p.fork == nil {
		fork, err := newFork(p)
		if err != nil {
			return err
		}
		p.fork = fork
	}

	p.cardsMeta = nil
	p.packages = map[string]string{}
	p.fset = token.NewFileSet()
	p.decls = make(map[string]*printer.CommentedNode)
	err := p.fork.syncAll()
	if err != nil {
		return newRetryError(e.lastError, err)
	}

	p.watcher, err = startWatcher(p.Dir, p.events)
	if err != nil {
		return newRetryError(e.lastError, err)
	}

	p.fatalError = nil
	p.events <- evRestartRunners{}
	return nil
}

type evStartRunner struct {
	devcardName string
	id          chan<- string
}

func (e evStartRunner) act(p *Project) error {
	var r *runner.Runner
	meta, err := p.findDevcardMeta(e.devcardName)
	if err == nil {
		err = p.generator.Run()
	}
	if err != nil {
		r = runner.StartFakeRunner(p.cfg, err)
	} else {
		r = runner.Start(p.cfg, p.fork.dir, meta)
	}
	p.runners[r] = struct{}{}
	e.id <- r.Id
	return nil
}

type evGetRunner struct {
	runnerId string
	updates  chan<- chan any
}

func (e evGetRunner) act(p *Project) error {
	for r := range p.runners {
		if r.Id == e.runnerId {
			e.updates <- r.Updates
			break
		}
	}
	close(e.updates)
	return nil
}

type evStopRunner struct {
	runnerId string
}

func (e evStopRunner) act(p *Project) error {
	for r := range p.runners {
		if r.Id == e.runnerId {
			r.Shutdown()
			delete(p.runners, r)
			break
		}
	}
	return nil
}

func (p *Project) findDevcardMeta(devcardName string) (devcard.DevcardMeta, error) {
	for _, meta := range p.cardsMeta {
		if meta.Name == devcardName {
			return meta, nil
		}
	}
	return devcard.DevcardMeta{}, fmt.Errorf("no such devcard in %s: %s", p.Name, devcardName)
}

type evFail struct {
	err error
}

// MUST return nil
func (e evFail) act(p *Project) error {
	p.fatalError = e.err
	p.events <- evRestartRunners{}
	return nil
}

type evShutdown struct {
	err chan<- error
}

func (e evShutdown) act(p *Project) error {
	for r := range p.runners {
		r.Shutdown()
	}
	p.runners = map[*runner.Runner]struct{}{}
	var errW, errF error
	if p.watcher != nil {
		errW = p.watcher.Close()
		p.watcher = nil
	}
	if p.fork != nil {
		errF = p.fork.delete()
		p.fork = nil
	}
	err := errors.Join(errW, errF)
	if err != nil {
		err = fmt.Errorf("shutting down project %s: %w", p.Dir, err)
	}
	e.err <- err
	close(e.err)
	return err
}
