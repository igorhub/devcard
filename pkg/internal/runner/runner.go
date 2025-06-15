package runner

// notes: we don't call process.Kill (and send "exit" instead), because Kill doesn't kill potential subprocesses.

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/igorhub/devcard"
	"github.com/igorhub/devcard/pkg/internal/config"
	"github.com/igorhub/devcard/pkg/internal/render"
)

type Runner struct {
	ch           chan any // maybe not
	dir          string
	transientDir string
	cardMeta     devcard.DevcardMeta
	cfg          *config.Config

	start, build int

	Id          string
	DevcardName string
	Error       error
	Updates     chan any
}

func StartFakeRunner(cfg *config.Config, err error) *Runner {
	r := &Runner{
		cfg:     cfg,
		Id:      "r" + strconv.Itoa(rand.Int()),
		ch:      make(chan any, 1024),
		Updates: make(chan any, 8),
		Error:   err,
	}
	r.Updates <- Card{}
	r.Updates <- makeError(err)
	// r.Updates <- Cell{"-dc-cell-error", "restart the server?"}
	return r
}

func Start(cfg *config.Config, dir string, meta devcard.DevcardMeta) *Runner {
	r := &Runner{
		cfg:          cfg,
		Id:           "r" + strconv.Itoa(rand.Int()),
		ch:           make(chan any, 1024),
		dir:          dir,
		transientDir: filepath.Join(dir, "_transient"+strconv.Itoa(rand.Int())),
		cardMeta:     meta,
		DevcardName:  meta.Name,
		Updates:      make(chan any, 1024),
	}
	if err := os.Mkdir(r.transientDir, 0700); err != nil {
		r.Error = fmt.Errorf("unable to start runner in %s: %w", dir, err)
	}

	go r.runEventLoop()
	return r
}

func (r *Runner) Restart(cfg *config.Config, err error) {
	r.cfg = cfg
	r.ch <- evRestart{err}
}

func (r *Runner) Shutdown() {
	r.ch <- evClose{}
}

func makeError(err error) Error {
	title := "Fatal error"
	return Error{title, err}
}

func (r *Runner) runEventLoop() {
	var cache *Card
	// TODO: cfg.Appearance.CodeHighlighting
	for {
		ctx, cancel := context.WithCancel(context.Background())
		var started, built, finished int64
		started = time.Now().UnixMilli()
		highlighter := render.NewHighlighter(r.cfg.Appearance.CodeHighlighting)

		if r.Error == nil {
			ch := r.ch
			go func() {
				ch <- Heartbeat{}
				ch <- CSS{Values: []string{devcard.CSSFromServer}}
				r.run(ctx, ch)
				ch <- evFlush{}
				ch <- evFinish{}
			}()
			go func() {
				time.Sleep(1000 * time.Millisecond)
				ch <- evFlush{}
			}()
		} else {
			r.ch <- makeError(r.Error)
		}
	innerLoop:
		for e := range r.ch {
			// log.Printf("[runner %s] %#v\n", r.Id, e)
			switch x := e.(type) {
			case evRestart:
				cache = newCard()
				r.ch = make(chan any, 1024)
				r.Error = x.err
				break innerLoop

			case evClose:
				cancel()
				close(r.Updates)
				return

			case evBuilt:
				built = time.Now().UnixMilli()
				r.Updates <- Meta{BuildTime: formatTime(built - started)}

			case evFinish:
				finished = time.Now().UnixMilli()
				r.Updates <- Meta{RunTime: formatTime(finished - built)}

			case Heartbeat:
				now := time.Now().UnixMilli()
				switch {
				case built == 0:
					r.Updates <- Meta{BuildTime: formatTime(now - started)}
				case finished == 0:
					r.Updates <- Meta{RunTime: formatTime(now - built)}
				default:
					r.Updates <- Heartbeat{}
					break
				}
				ch := r.ch
				sleep := 100 * time.Millisecond
				if finished != 0 || now-started > 90000 {
					sleep = 500 * time.Millisecond
				}
				go func() {
					time.Sleep(sleep)
					ch <- Heartbeat{}
				}()

			case Title:
				r.Updates <- e

			case CSS:
				x.makeStylesheet(*r.cfg)
				r.Updates <- x

			case evCell:
				html := render.RenderCell(highlighter, x.Cell)
				if cache != nil {
					cache.addCell(Cell{x.Id, html})
				} else {
					r.Updates <- Cell{x.Id, html}
				}

			case Error:
				r.ch <- evFlush{x}

			case Stdout:
				if cache != nil {
					cache.Stdout += x.Line
				} else {
					r.Updates <- e
				}

			case Stderr:
				if cache != nil {
					cache.Stderr += x.Line
				} else {
					r.Updates <- e
				}

			case evFlush:
				if cache != nil {
					r.Updates <- *cache
					cache = nil
				}
				if x.withError.Err != nil {
					r.Updates <- x.withError
				}

			default:
				fmt.Printf("[runner]: received %T\n", e)
			}
		}
		cancel()
	}
}

func formatTime(ms int64) string {
	switch {
	case ms < 1000:
		return fmt.Sprintf("%dms", ms)
	case ms < 90000:
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	default:
		h := ms / (60 * 60 * 1000)
		m := (ms - h*60*60*1000) / (60 * 1000)
		s := (ms - h*60*60*1000 - m*60*1000) / 1000
		ret := fmt.Sprintf("%d:%02d", m, s)
		if h > 0 {
			ret = fmt.Sprintf("%d:%02d:%02d", h, m, s)
		}
		return ret
	}
}

type evRestart struct {
	err error
}

func (evRestart) updateMessage() {}

type evClose struct{}

type (
	evBuilt  struct{}
	evFinish struct{}
	evFlush  struct{ withError Error }
)

type evCell struct {
	Id   string
	Cell devcard.Cell
}

func (evBuilt) updateMessage()  {}
func (evFinish) updateMessage() {}
func (evFlush) updateMessage()  {}
func (evCell) updateMessage()   {}
