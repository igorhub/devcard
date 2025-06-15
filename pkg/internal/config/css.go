package config

import (
	_ "embed"
	"log"
	"os"
	"strings"

	"github.com/igorhub/devcard/pkg/internal/render"
)

//go:embed css/new.css
var newCSS []byte

//go:embed css/light.css
var lightTheme []byte

//go:embed css/dark.css
var darkTheme []byte

//go:embed css/gruvbox-light.css
var gruvboxLightTheme []byte

//go:embed css/gruvbox-dark.css
var gruvboxDarkTheme []byte

func (cfg Config) CSS() string {
	s := new(strings.Builder)
	for _, stylesheet := range cfg.Appearance.Stylesheets {
		switch stylesheet {
		case "builtin":
			s.Write(newCSS)
		case "builtin/light":
			s.Write(lightTheme)
		case "builtin/dark":
			s.Write(darkTheme)
		case "builtin/gruvbox-light":
			s.Write(gruvboxLightTheme)
		case "builtin/gruvbox-dark":
			s.Write(gruvboxDarkTheme)
		default:
			data, err := os.ReadFile(stylesheet)
			if err != nil {
				log.Println("Can't read CSS file:", err)
				break
			}
			s.Write(data)
		}
	}
	if cfg.Appearance.CodeHighlighting != "" {
		s.Write(render.NewHighlighter(cfg.Appearance.CodeHighlighting).CSS())
	}
	return s.String()
}
