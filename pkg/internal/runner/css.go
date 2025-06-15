package runner

import (
	"log"
	"os"
	"strings"

	"github.com/igorhub/devcard"
	"github.com/igorhub/devcard/pkg/internal/config"
	"github.com/igorhub/devcard/pkg/internal/file"
)

func (css *CSS) makeStylesheet(cfg config.Config) {
	s := new(strings.Builder)
	for _, v := range css.Values {
		switch {
		case v == devcard.CSSFromServer:
			s.WriteString(cfg.CSS())
		case file.Exists(v):
			data, err := os.ReadFile(v)
			if err != nil {
				log.Println("Error in makeCSS:", err)
			} else {
				s.Write(data)
			}
		default:
			s.WriteString(v)
		}
		s.WriteString("\n")
	}
	css.Stylesheet = s.String()
}
