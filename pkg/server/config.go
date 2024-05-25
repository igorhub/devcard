package server

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Path string
	Data []byte
	Err  error

	Port int

	Editor string
	Opener string `toml:"custom-opener"`

	Projects []projectConfig

	Appearance struct {
		Stylesheets      []string
		CodeHighlighting string `toml:"code-highlighting"`
	}
}

type projectConfig struct {
	Name   string
	Dir    string
	Inject string
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
			Dir    string
			Inject string
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
		cfg.Projects = append(cfg.Projects, projectConfig{
			Name:   name,
			Dir:    p.Dir,
			Inject: p.Inject,
		})
	}

	index := func(projectName string) int {
		s := []string{"project", projectName}
		return slices.IndexFunc(meta.Keys(), func(k toml.Key) bool {
			return slices.Compare(k, s) == 0
		})
	}
	slices.SortFunc(cfg.Projects, func(a, b projectConfig) int {
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
	return os.WriteFile(cfg.Path, []byte(s), 0664)
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
	project := projectRoot(cwd)
	if project != "" {
		cfg.Projects = append(cfg.Projects, projectConfig{
			Name: filepath.Base(project),
			Dir:  project,
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
