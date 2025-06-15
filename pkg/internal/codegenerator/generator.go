package codegenerator

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/igorhub/devcard/pkg/internal/config"
	"github.com/igorhub/devcard/pkg/internal/file"
)

type Generator struct {
	projectDir string
	generators map[string]generator
	files      map[string]struct{}
}

func New(cfg config.ProjectConfig) *Generator {
	g := &Generator{
		projectDir: cfg.Dir,
		files:      map[string]struct{}{},
		generators: map[string]generator{},
	}

	for extensions, cmdWithArgs := range cfg.Generators {
		if len(cmdWithArgs) == 0 {
			continue
		}
		cmd, args := cmdWithArgs[0], cmdWithArgs[1:]
		if strings.HasPrefix(cmd, "~/") {
			homeDir, err := os.UserHomeDir()
			if err == nil {
				cmd = filepath.Join(homeDir, cmd[2:])
			}
		}
		for _, ext := range strings.Split(extensions, ";") {
			ext = strings.TrimSpace(ext)
			g.generators[ext] = generator{cmd, args}
		}
	}

	return g
}

func (g *Generator) AddFile(path string) {
	g.files[path] = struct{}{}
}

func (g *Generator) Run() error {
	var errs []error
	for f := range g.files {
		if generator, ok := g.generators[filepath.Ext(f)]; ok {
			err := generator.run(g.projectDir, f)
			if err != nil && !file.Exists(f) {
				err = nil
			}
			if err == nil {
				delete(g.files, f)
			}
			errs = append(errs, err)
		}
	}

	if generator, ok := g.generators[""]; ok {
		errs = append(errs, generator.run(g.projectDir, ""))
	}

	return errors.Join(errs...)
}

type generator struct {
	cmd  string
	args []string
}

func (g *generator) commandLine(file string) []string {
	ret := []string{g.cmd}
	for _, arg := range g.args {
		if arg == "$file" {
			ret = append(ret, file)
		} else {
			ret = append(ret, arg)
		}
	}
	return ret
}

func (g *generator) run(projectDir string, file string) error {
	cl := g.commandLine(file)
	cmd := exec.Command(cl[0], cl[1:]...)
	cmd.Dir = projectDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w\n\n%s", strings.Join(cl, " "), err, string(out))
	}
	return nil
}
