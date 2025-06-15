package config

import (
	"cmp"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"

	"github.com/BurntSushi/toml"
	"github.com/igorhub/devcard/pkg/internal/file"
)

type Config struct {
	Path string
	Data []byte
	Err  error

	Port int

	Editor string
	Opener string `toml:"custom-opener"`

	Projects []ProjectConfig

	Appearance struct {
		Stylesheets      []string
		CodeHighlighting string `toml:"code-highlighting"`
	}
}

type ProjectConfig struct {
	Name       string
	Dir        string
	Injection  string
	Generators map[string][]string
}

func configPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "devcards", "devcards.toml"), nil
}

func LoadConfig() Config {
	cfg := defaultConfig()
	if cfg.Err != nil {
		return cfg
	}

	path, err := configPath()
	if err != nil {
		cfg.Err = err
		return cfg
	}

	cfg.readConfig(path)
	return cfg
}

func (cfg *Config) readConfig(path string) {
	cfg.Path = path
	cfg.Data, cfg.Err = os.ReadFile(path)
	if cfg.Err != nil {
		_, err := os.Stat(path)
		if err != nil && errors.Is(err, fs.ErrNotExist) {
			cfg.Err = err
		}
		return
	}

	cfg.Err = toml.Unmarshal(cfg.Data, cfg)
	if cfg.Err != nil {
		return
	}

	cfg.Err = cfg.readProjects()
}

// readProjects reads the projects from cfg.Data and puts them in the same order
// they are described in the config file.
func (cfg *Config) readProjects() error {
	var x struct {
		Project map[string]struct {
			Dir        string
			Inject     string              `toml:"inject-code"`
			Generators map[string][]string `toml:"code-generators"`
		}
	}
	meta, err := toml.Decode(string(cfg.Data), &x)
	if err != nil {
		return err
	}

	if len(x.Project) != 0 {
		cfg.Projects = cfg.Projects[:0]
	}

	for name, p := range x.Project {
		pc := ProjectConfig{
			Name:       name,
			Dir:        p.Dir,
			Injection:  p.Inject,
			Generators: p.Generators,
		}
		cfg.Projects = append(cfg.Projects, pc)
	}

	index := func(projectName string) int {
		s := []string{"project", projectName}
		return slices.IndexFunc(meta.Keys(), func(k toml.Key) bool {
			return slices.Compare(k, s) == 0
		})
	}
	slices.SortFunc(cfg.Projects, func(a, b ProjectConfig) int {
		return cmp.Compare(index(a.Name), index(b.Name))
	})
	return nil
}

func (cfg *Config) Create() error {
	var projectsStr string
	if len(cfg.Projects) > 0 {
		for _, project := range cfg.Projects {
			projectsStr += fmt.Sprintf("\n[project.%s]\ndir = \"%s\"\n", project.Name, project.Dir)
		}
	}

	format := `port = %d
editor = "vscode"

[appearance]
# Builtin styles:
# * builtin/light
# * builtin/dark
# * builtin/gruvbox-light
# * builtin/gruvbox-dark
stylesheets = ["builtin", "builtin/light"]

# Highlighting styles are listed here: https://xyproto.github.io/splash/docs/all.html
code-highlighting = "tango"
%s
# [project.name-of-your-project]
# dir = "/absolute/path/to/your/project"
`
	s := fmt.Sprintf(format, cfg.Port, projectsStr)

	os.Mkdir(filepath.Dir(cfg.Path), 0775)
	createGenerators(filepath.Dir(cfg.Path))
	return os.WriteFile(cfg.Path, []byte(s), 0664)
}

//go:embed generators
var generatorsFS embed.FS

func createGenerators(configDir string) {
	files, _ := generatorsFS.ReadDir("generators")
	for _, f := range files {
		out := filepath.Join(configDir, f.Name())
		if file.Exists(out) {
			continue
		}
		data, _ := fs.ReadFile(generatorsFS, "generators/"+f.Name())
		if err := os.WriteFile(out, data, 0777); err != nil {
			log.Printf("Failed to write a generator file (%s): %s\n", out, err)
		}
	}
}

func defaultConfig() Config {
	cfg := Config{
		Port:   50051,
		Editor: "vscode",
	}
	cfg.Appearance.Stylesheets = []string{"builtin", "builtin/light"}
	cfg.Appearance.CodeHighlighting = "tango"

	cwd, err := os.Getwd()
	if err != nil {
		cfg.Err = err
		return cfg
	}
	projectDir := projectRoot(cwd)
	if projectDir != "" {
		cfg.Projects = append(cfg.Projects, ProjectConfig{
			Name: filepath.Base(projectDir),
			Dir:  projectDir,
		})
	}

	return cfg
}

func projectRoot(dir string) string {
	mod := filepath.Join(dir, "go.mod")
	if _, err := os.Stat(mod); err == nil {
		return dir
	}

	parent := filepath.Dir(dir)
	if parent == dir {
		return ""
	}
	return projectRoot(parent)
}
