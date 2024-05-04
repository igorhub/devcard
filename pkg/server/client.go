package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/igorhub/devcard"
	"github.com/igorhub/devcard/pkg/internal/project"
)

type clientEvent int

const (
	refresh clientEvent = iota
	halt
)

type client struct {
	id         string
	url        string
	conn       *websocket.Conn
	websocketC chan []byte

	project      *project.Project
	repo         *project.Repo
	updateFn     func(context.Context, chan<- []byte)
	unregisterFn func()
	highlighter  *highlighter

	executionC chan clientEvent
	unblock    string
	unblockC   chan struct{}
}

func newClient(cfg Config, id, url string, project *project.Project) *client {
	c := &client{
		id:          id,
		url:         url,
		websocketC:  make(chan []byte, 256),
		executionC:  make(chan clientEvent, 256),
		project:     project,
		highlighter: newHighlighter(cfg.Appearance.CodeHighlighting),
	}
	return c
}

func msgClear() []byte {
	data, _ := json.Marshal(map[string]string{
		"msgType": "clear",
	})
	return data
}

func msgSaveScrollPosition() []byte {
	data, _ := json.Marshal(map[string]string{
		"msgType": "saveScrollPosition",
	})
	return data
}

func msgRestoreScrollPosition() []byte {
	data, _ := json.Marshal(map[string]string{
		"msgType": "restoreScrollPosition",
	})
	return data
}

func msgJump(id string) []byte {
	data, _ := json.Marshal(map[string]string{
		"msgType": "jump",
		"id":      id,
	})
	return data
}

func msgSetTitle(title string) []byte {
	data, _ := json.Marshal(map[string]string{
		"msgType": "setTitle", "title": title,
	})
	return data
}

func msgAppendCell(id string, html string) []byte {
	data, _ := json.Marshal(map[string]string{
		"msgType": "appendCell",
		"cellId":  id,
		"html":    html,
	})
	return data
}

func msgSetCellContent(id string, html string) []byte {
	data, _ := json.Marshal(map[string]string{
		"msgType": "setCellContent",
		"cellId":  id,
		"html":    html,
	})
	return data
}

func msgAppendToCell(id string, html string) []byte {
	data, _ := json.Marshal(map[string]string{
		"msgType": "appendToCell",
		"cellId":  id,
		"html":    html,
	})
	return data
}

func msgSetStatusBarContent(items ...string) []byte {
	data, _ := json.Marshal(map[string]string{
		"msgType": "setStatusBarContent",
		"html":    strings.Join(items, " "),
	})
	return data
}

func msgBatch(messages [][]byte) []byte {
	js := make([]json.RawMessage, len(messages))
	for i, msg := range messages {
		js[i] = json.RawMessage(msg)
	}
	data, _ := json.Marshal(map[string]any{
		"msgType":  "batch",
		"messages": js,
	})
	return data
}

func msgNop() []byte {
	return []byte("{\"msgType\": \"nop\"}")
}

func (c *client) reportProjectError(ch chan<- []byte) {
	ch <- msgClear()
	ch <- msgSetTitle("Error")

	b := devcard.NewErrorCell("Server error", c.project.Err.Error())
	ch <- msgAppendCell("error", renderCell(c.project, c.highlighter, b))

	restartUrl := "/restart?redirect=" + url.QueryEscape(c.url)
	md := fmt.Sprintf("Server cannot recover from this error. [Restart the server](%s).", restartUrl)
	ch <- msgAppendCell("restart", MdToHTML(md))

	close(ch)
}

func (c *client) connect(conn *websocket.Conn) {
	switch {
	case c.conn == nil: // User opened a new web page.
		c.conn = conn
		go c.runWebsocket()
		go c.runUpdates()
	case c.conn != nil: // User went back or forward in history.
		c.conn = conn
	}
}

func (c *client) close() {
	c.executionC <- halt
}

func (c *client) refresh() {
	c.executionC <- refresh
}

func (c *client) runUpdates() {
	var (
		ctx    context.Context
		cancel context.CancelFunc = func() {}
	)

	for {
		cmd := <-c.executionC
		cancel()
		if cmd == halt {
			c.conn.Close()
			return
		}

		ctx, cancel = context.WithCancel(context.Background())
		retranslator := make(chan []byte, 256)
		go func() {
			for {
				select {
				case msg, ok := <-retranslator:
					if !ok {
						return
					}
					c.websocketC <- msg
				case <-ctx.Done():
					for range retranslator {
					}
					return
				}
			}
		}()

		if c.project.Err != nil {
			c.reportProjectError(retranslator)
		} else {
			c.updateFn(ctx, retranslator)
		}
	}
}

func (c *client) runWebsocket() {
	for {
		msg, ok := <-c.websocketC
		if !ok {
			return
		}

		err := c.conn.WriteMessage(websocket.TextMessage, msg)
		if errors.Is(err, websocket.ErrCloseSent) {
			log.Printf("websocket: %s: Connection closed", c.name())
			c.unregisterFn()
			break
		} else if err != nil {
			log.Printf("websocket: %s: %s", c.name(), err)
			c.unregisterFn()
			break
		}
	}
}

func (c *client) name() string {
	name := c.project.Name
	if c.repo != nil {
		name += "—" + c.repo.DevcardInfo.Name
	}
	return name
}
