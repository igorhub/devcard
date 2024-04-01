package server

import (
	"fmt"
	"html"
	"net/url"
	"strings"

	"github.com/gomarkdown/markdown"
	mdhtml "github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"github.com/igorhub/devcard"
	"github.com/igorhub/devcard/pkg/internal/project"
)

func MdToHTML(md string) string {
	// create markdown parser with extensions
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock | parser.LaxHTMLBlocks
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse([]byte(md))

	// create HTML renderer with extensions
	htmlFlags := mdhtml.CommonFlags
	opts := mdhtml.RendererOptions{Flags: htmlFlags}
	renderer := mdhtml.NewRenderer(opts)

	return string(markdown.Render(doc, renderer))
}

func renderCell(project *project.Project, highlighter *highlighter, b devcard.Cell) string {
	switch b := b.(type) {
	case *devcard.MarkdownCell:
		return renderMarkdown(b)
	case *devcard.HTMLCell:
		return b.HTML
	case *devcard.ErrorCell:
		return renderError(b.Title, b.Body)
	case *devcard.MonospaceCell:
		return renderMonospace(highlighter, b)
	case *devcard.ValueCell:
		return renderValue(highlighter, b)
	case *devcard.AnnotatedValueCell:
		return renderAnnotatedValue(highlighter, b)
	case *devcard.SourceCell:
		return renderSource(project, highlighter, b)
	case *devcard.ImageCell:
		return renderImage(b)
	case *devcard.JumpCell:
		return ""
	case nil:
		return renderError("Rendering error: trying to render nil", "")
	default:
		return renderError(fmt.Sprintf("Rendering error: unknown type '%s'", b.Type()), "")
	}
}

func renderError(title, body string) string {
	if title == "" && body == "" {
		return ""
	}
	result := fmt.Sprintf(`<div class="err">` + title + `</div>`)
	if body != "" {
		result += fmt.Sprintf(`<pre class="err">` + html.EscapeString(body) + `</pre>`)
	}
	return result
}

func renderMarkdown(b *devcard.MarkdownCell) string {
	return MdToHTML(b.Text)
}

func renderMonospace(highlighter *highlighter, b *devcard.MonospaceCell) string {
	if b.Text == "" {
		return ""
	}

	result := "<pre><code>" + html.EscapeString(b.Text) + "</code></pre>"
	if b.Highlighting != "" {
		h, err := highlighter.Highlight(b.Text, b.Highlighting)
		if err == nil {
			result = h
		}
	}
	return result
}

func renderSource(project *project.Project, highlighter *highlighter, b *devcard.SourceCell) string {
	if b.Decl == "" {
		return ""
	}
	s, err := project.Source(b.Decl)
	if err != nil {
		return renderError("SourceCell error", err.Error())
	}
	return renderMonospace(highlighter, devcard.NewMonospaceCell(s, devcard.WithHighlighting("go")))
}

func renderValue(highlighter *highlighter, b *devcard.ValueCell) string {
	values := strings.Join(b.Values, "\n\n")
	result, err := highlighter.Highlight(values, "go")
	if err != nil {
		result = "<pre><code>" + html.EscapeString(values) + "</code></pre>"
	}
	return result
}

func renderAnnotatedValue(highlighter *highlighter, b *devcard.AnnotatedValueCell) string {
	if len(b.AnnotatedValues) == 0 {
		return ""
	}
	s := new(strings.Builder)
	for i, v := range b.AnnotatedValues {
		if i > 0 {
			s.WriteString("\n\n")
		}
		if v.Annotation != "" {
			s.WriteString(fmt.Sprintf("// %s\n", v.Annotation))
		}
		s.WriteString(v.Value)
	}

	result, err := highlighter.Highlight(s.String(), "go")
	if err != nil {
		result = "<pre><code>" + html.EscapeString(s.String()) + "</code></pre>"
	}
	return result
}

func renderImage(b *devcard.ImageCell) string {
	if b.Error != nil {
		return renderError(b.Error.Title, b.Error.Body)
	}

	f := `<figure>
  <img
  src="/file?path=%s"
  alt="%s"/>
  <figcaption>%s</figcaption>
</figure>
`

	s := &strings.Builder{}
	for _, img := range b.Images {
		fmt.Fprintf(s, f, url.QueryEscape(img.Path), img.Path, img.Annotation)
	}
	return s.String()
}
