package server

import (
	"context"
	"fmt"
	"html"
	"slices"
	"sync"
	"time"

	"github.com/igorhub/devcard"
	"github.com/igorhub/devcard/pkg/internal/project"
)

func (c *client) navigationBlock() string {
	var prevLink, upLink, nextLink string
	upLink = fmt.Sprintf("[top: %s](/dc/%s?jump=%s)", c.repo.DevcardInfo.Package, c.project.Name, pkgId(c.repo.DevcardInfo))

	cardsInfo := c.project.DevcardsInfo()
	cardsInfo = slices.DeleteFunc(cardsInfo, func(info devcard.DevcardInfo) bool {
		return info.ImportPath != c.repo.DevcardInfo.ImportPath
	})
	if len(cardsInfo) < 2 {
		return MdToHTML("❬ " + upLink + " ❭")
	}
	prevIdx, nextIdx := len(cardsInfo)-1, 0
	for i := range cardsInfo {
		if i > 0 && cardsInfo[i-1].Name == c.repo.DevcardInfo.Name {
			nextIdx = i
		}
		if i < len(cardsInfo)-1 && cardsInfo[i+1].Name == c.repo.DevcardInfo.Name {
			prevIdx = i
		}
	}

	prevLink = fmt.Sprintf("[prev: %s](%s)", cardsInfo[prevIdx].Caption(), cardsInfo[prevIdx].Name)
	nextLink = fmt.Sprintf("[next: %s](%s)", cardsInfo[nextIdx].Caption(), cardsInfo[nextIdx].Name)
	return MdToHTML("❬ " + prevLink + " | " + upLink + " | " + nextLink + " ❭")
}

func (c *client) initDevcard(devcardName string, unregisterFn func()) {
	c.unregisterFn = unregisterFn

	var err error
	c.repo, err = c.project.CreateRepo(devcardName)
	if err != nil {
		c.initError(err, unregisterFn)
		return
	}

	title := c.repo.DevcardInfo.Caption()
	c.updateFn = func(ctx context.Context, ch chan<- []byte) {
		ch <- msgSetTitle(title)
		ch <- msgSetCellContent("-devcard-navigation", c.navigationBlock())

		var (
			buildTime   int64
			runTime     int64 = -1
			failedTests int
		)
		setBadges := func() {
			var badges []string
			badges = append(badges, "<code>build: "+formatTime(buildTime)+"</code>")
			if runTime >= 0 {
				badges = append(badges, "<code>run: "+formatTime(runTime)+"</code>")
			}
			if failedTests == 1 {
				badges = append(badges, `<code class="err">1 test failed</code>`)
			} else if failedTests > 1 {
				badges = append(badges, fmt.Sprintf(`<code class="err">%d tests failed</code>`, failedTests))
			}
			ch <- msgSetStatusBarContent(badges...)
		}

		wg := sync.WaitGroup{}
		wg.Add(2)
		buildC := make(chan struct{})
		go func() {
			defer wg.Done()
			init := time.Now()
			for {
				select {
				case <-buildC:
					return
				case <-ctx.Done():
					return
				default:
				}
				buildTime = (time.Since(init).Milliseconds() / 100) * 100
				setBadges()
				time.Sleep(100 * time.Millisecond)
			}
		}()

		runC := make(chan struct{})
		go func() {
			defer wg.Done()
			<-buildC
			init := time.Now()
			for {
				select {
				case <-runC:
					return
				case <-ctx.Done():
					return
				default:
				}
				runTime = (time.Since(init).Milliseconds() / 100) * 100
				setBadges()
				time.Sleep(100 * time.Millisecond)
			}
		}()

		control := make(chan string)
		updates := make(chan project.UpdateMessage, 4096)
		go c.repo.Run(ctx, control, updates)

		go func() {
			init := time.Now()
			<-updates
			close(buildC)
			buildTime = time.Since(init).Milliseconds()
			init = time.Now()
			processUpdates(c, ch, control, updates)
			close(runC)
			runTime = time.Since(init).Milliseconds()
			failedTests = c.repo.Test(ctx)
			setBadges()
			wg.Wait()
			close(ch)
		}()
	}
}

const warmupTime = 1000 * time.Millisecond

var breakBatching []byte = []byte("break batching")

func makeBatcher(websocketC chan<- []byte) (chan []byte, chan struct{}) {
	ch := make(chan []byte)
	done := make(chan struct{})

	var batch [][]byte
	go func() {
		batching := true
		stopBatching := time.NewTimer(warmupTime)
	loop:
		for {
			var msg []byte
			var ok bool
			select {
			case msg, ok = <-ch:
				if !ok {
					break loop
				}
				if slices.Compare(msg, breakBatching) == 0 {
					batching = false
					msg = msgNop()
				}
			case <-stopBatching.C:
				batching = false
				msg = msgNop()
			}

			if batching {
				batch = append(batch, msg)
			} else if len(batch) > 0 {
				batch = append(batch, msg)
				websocketC <- msgBatch(batch)
				batch = nil
			} else {
				websocketC <- msg
			}
		}
		if len(batch) > 0 {
			websocketC <- msgBatch(batch)
		}
		close(done)
	}()

	return ch, done
}

func processUpdates(c *client, ch chan<- []byte, control chan<- string, updates <-chan project.UpdateMessage) {
	var stdoutCellCreated, stderrCellCreated bool

	ch, done := makeBatcher(ch)
	defer func() {
		close(ch)
		<-done
	}()

	var wg sync.WaitGroup
	defer wg.Wait()

	ch <- msgClear()
	for update := range updates {
		switch x := update.(type) {

		case project.MsgInfo:
			if x.Title != "" {
				ch <- msgSetTitle(x.Title)
			}

		case project.MsgCell:
			ch <- msgAppendCell(x.Id, renderCell(c.project, c.highlighter, x.Cell))

			if cell, ok := x.Cell.(*devcard.JumpCell); ok {
				wg.Add(1)
				go func(cell *devcard.JumpCell) {
					time.Sleep(time.Duration(cell.Delay) * time.Millisecond)
					ch <- msgJump(x.Id)
					wg.Done()
				}(cell)
			}

			if cell, ok := x.Cell.(*devcard.WaitCell); ok {
				ch <- breakBatching
				c.unblock = cell.Id
				c.unblockC = make(chan struct{})
				go func() {
					<-c.unblockC
					control <- "unblock " + cell.Id
				}()
			}

		case project.MsgError:
			b := devcard.NewErrorCell(x.Title)
			if x.Err != nil {
				b.Append(x.Err)
			}
			ch <- msgAppendCell(fmt.Sprintf("%p", &x), renderCell(c.project, c.highlighter, b))

		case project.MsgPipeOut:
			switch x.Pipe {
			case project.PipeStdout:
				if !stdoutCellCreated {
					stdoutCellCreated = true
					s := `<h3>Stdout:</h3><pre id="-devcard-stdout-cell"></pre>`
					ch <- msgSetCellContent("-devcard-stdout", s)
				}
				ch <- msgAppendToCell("-devcard-stdout-cell", html.EscapeString(x.Line))
			case project.PipeStderr:
				if !stderrCellCreated {
					stderrCellCreated = true
					s := `<h3 class="err">Stderr:</h3><pre id="-devcard-stderr-cell"></pre>`
					ch <- msgSetCellContent("-devcard-stderr", s)
				}
				ch <- msgAppendToCell("-devcard-stderr-cell", html.EscapeString(x.Line))
			}
		}
	}
}

func formatTime(duration int64) string {
	if duration < 1000 {
		return fmt.Sprintf("%dms", duration)
	} else {
		return fmt.Sprintf("%.1fs", float64(duration)/1000)
	}
}
