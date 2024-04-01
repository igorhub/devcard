package server

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/igorhub/devcard"
)

func pkgId(info devcard.DevcardInfo) string {
	return filepath.Base(filepath.Dir(info.ImportPath)) + "-" + filepath.Base(info.ImportPath)
}

func (c *client) initListDevcards(unregisterFn func()) {
	c.unregisterFn = unregisterFn

	var jump string
	if q, _ := url.Parse(c.url); q != nil {
		jump = q.Query().Get("jump")
	}

	c.updateFn = func(_ context.Context, ch chan<- []byte) {
		b := new(bytes.Buffer)
		cardsInfo := c.project.DevcardsInfo()
		for i, info := range cardsInfo {
			newSection := i == 0 || info.ImportPath != cardsInfo[i-1].ImportPath
			if newSection {
				id := pkgId(info)
				fmt.Fprintf(b, "\n<h4 id=\"%s\">%s <span class=\"import-path\">%s</span></h4>\n", id, info.Package, info.ImportPath)
			}

			link := "/dc/" + c.project.Name + "/" + info.Name
			fmt.Fprintf(b, "* [%s](%s)\n", info.Caption(), link)
		}
		html := MdToHTML(b.String())

		ch <- msgAppendCell("list", html)
		if jump != "" {
			ch <- msgJump(jump)
			jump = ""
		}

		close(ch)
	}
}
