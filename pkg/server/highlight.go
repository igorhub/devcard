package server

import (
	"bytes"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

type highlighter struct {
	style     *chroma.Style
	formatter *html.Formatter
	css       []byte
}

func newHighlighter(style string) *highlighter {
	return &highlighter{
		style:     styles.Get(style),
		formatter: html.New(html.WithClasses(true), html.ClassPrefix("-devcard-hl-")),
	}
}

func (h *highlighter) CSS() []byte {
	out := new(bytes.Buffer)
	err := h.formatter.WriteCSS(out, h.style)
	if err != nil {
		return nil
	}
	return out.Bytes()
}

func (h *highlighter) Highlight(src, lang string) (string, error) {
	lexer := lexers.Get(lang)
	iterator, err := lexer.Tokenise(nil, src)
	if err != nil {
		return "", err
	}

	out := new(bytes.Buffer)
	err = h.formatter.Format(out, h.style, iterator)
	if err != nil {
		return "", err
	}

	return out.String(), nil
}
