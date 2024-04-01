package server

import (
	"context"
	"html"
)

func (c *client) initError(err error, unregisterFn func()) {
	c.unregisterFn = unregisterFn
	c.updateFn = func(_ context.Context, ch chan<- []byte) {
		ch <- msgClear()
		ch <- msgSetTitle("Error")
		ch <- msgAppendCell("error", "<code>"+html.EscapeString(err.Error())+"</code>")
		close(ch)
	}
}
