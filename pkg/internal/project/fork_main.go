package project

import (
	"bytes"
	_ "embed"
	"fmt"
	"hash/maphash"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/igorhub/devcard"
	"github.com/igorhub/devcard/pkg/internal/file"
)

var devcardMainTemplate = makeDevcardMainTemplate()

//go:embed devcard_main.template
var templateText string

func makeDevcardMainTemplate() *template.Template {
	result := template.New("devcard_main.template")
	result.Funcs(template.FuncMap{"producerName": producerName})
	result, err := result.Parse(templateText)
	if err != nil {
		panic(err)
	}
	return result
}

func producerName(devcard string) string {
	return strings.TrimPrefix(devcard, "main.")
}

var hashSeed = maphash.MakeSeed()

func (f *fork) generateMains() error {
	for _, cards := range f.p.cardsMeta.GroupByImportPath() {
		dir := filepath.Join(f.dir, file.DevcardMainDir(cards[0]))
		os.Mkdir(dir, 0775)
		err := os.WriteFile(filepath.Join(dir, "gen_devcard_main.go"), devcardMain(cards), 0664)
		if err != nil {
			return fmt.Errorf("generate main: %w", err)
		}
	}
	return nil
}

// devcardMain generates a Go source for the file with the main function.
func devcardMain(cards []devcard.DevcardMeta) []byte {
	data := struct {
		Cards        DevcardsMetaSlice
		MaybeImport  string
		MaybePackage string
	}{Cards: cards}
	if cards[0].Package != "main" {
		data.MaybeImport = fmt.Sprintf("dc \"%s\"", cards[0].ImportPath)
		data.MaybePackage = "dc."
	}

	var b bytes.Buffer
	err := devcardMainTemplate.Execute(&b, data)
	if err != nil {
		panic(fmt.Errorf("devcardMainTemplate failed to execute: %w", err))
	}
	return b.Bytes()
}
