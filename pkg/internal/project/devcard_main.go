package project

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	"github.com/igorhub/devcard"
)

const (
	generatedMainDir  = "generated_devcard_main"
	generatedMainFile = "generated_devcard_main.go"
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

type templateData = struct {
	Import string
	Card   string
}

// devcardMain generates a Go source for the file with the main function.
func devcardMain(info devcard.DevcardInfo) []byte {
	data := templateData{}
	if info.Package != "main" {
		data.Import = fmt.Sprintf("dc \"%s\"", info.ImportPath)
		data.Card = "dc." + info.Name
	} else {
		data.Card = info.Name
	}

	var b bytes.Buffer
	err := devcardMainTemplate.Execute(&b, data)
	if err != nil {
		panic(fmt.Errorf("devcardMainTemplate failed to execute: %w", err))
	}
	return b.Bytes()
}

func FindMainDir(info devcard.DevcardInfo) string {
	// If the devcard is located in a main package, our main function must be
	// placed in the same director.
	if info.Package == "main" {
		return filepath.Dir(info.Path)
	}

	// If a devcard path is located in an internal directory, our main
	// package must be placed at its root.
	parts := splitPath(info.Path)
	if i := indexLast(parts, "internal"); i > 0 {
		parts = append(parts[:i+1], generatedMainDir)
		return filepath.Join(parts...)
	}

	// Otherwise, we may create the directory for our main package anywhere;
	// we'll use a new directory in the root of the repo.
	return generatedMainDir
}

func GenerateMain(projectDir string, info devcard.DevcardInfo) error {
	dir := filepath.Join(projectDir, FindMainDir(info))
	os.Mkdir(dir, 0775)
	err := os.WriteFile(filepath.Join(dir, generatedMainFile), devcardMain(info), 0664)
	if err != nil {
		return fmt.Errorf("generate main: %w", err)
	}
	return nil
}

func splitPath(path string) []string {
	var result []string
	for {
		var last string
		path, last = filepath.Dir(path), filepath.Base(path)
		result = append(result, last)
		if last == path {
			break
		}
	}
	slices.Reverse(result)
	return result
}

func indexLast(s []string, v string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == v {
			return i
		}
	}
	return -1
}
