package server

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/igorhub/devcard/pkg/internal/config"
	"github.com/igorhub/devcard/pkg/internal/project"
	"github.com/igorhub/devcard/pkg/internal/runner"
	datastar "github.com/starfederation/datastar/sdk/go"
)

type server struct {
	cfg     config.Config
	handler http.Handler

	projects map[string]*project.Project
}

func NewServer(cfg config.Config) *server {
	s := &server{
		cfg:      cfg,
		projects: make(map[string]*project.Project),
	}

	for _, cfgProject := range cfg.Projects {
		p := project.NewProject(&cfg, cfgProject)
		s.projects[cfgProject.Name] = p
	}

	mux := http.NewServeMux()
	s.addRoutes(mux, cfg)
	s.handler = mux
	return s
}

func (s *server) restart() {
	s.cfg = config.LoadConfig()
	restartedProjects := map[string]bool{}
	for _, project := range s.projects {
		restartedProjects[project.Name] = true
		project.Restart(&s.cfg)
	}

	for _, cfgProject := range s.cfg.Projects {
		if _, ok := restartedProjects[cfgProject.Name]; !ok {
			p := project.NewProject(&s.cfg, cfgProject)
			s.projects[cfgProject.Name] = p
		} else {
			delete(restartedProjects, cfgProject.Name)
		}
	}

	for projectToShutdown := range restartedProjects {
		s.projects[projectToShutdown].Shutdown()
		delete(s.projects, projectToShutdown)
	}
}

func (s *server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s.handler.ServeHTTP(w, req)
}

func (s *server) Shutdown() {
	done := make(chan struct{})
	go func() {
		for _, project := range s.projects {
			log.Println("Shutting down " + project.Name)
			project.Shutdown()
		}
		close(done)
	}()

	select {
	case <-done:
		return
	case <-time.Tick(3 * time.Second):
		log.Println("Unable to shut down gracefully")
		os.Exit(1)
	}
}

//go:embed assets
var assetsFS embed.FS

func (s *server) addRoutes(mux *http.ServeMux, cfg config.Config) {
	mux.HandleFunc("GET /devcards", s.handleHomePage)
	mux.HandleFunc("GET /devcards/{project}", s.handleProject)
	mux.HandleFunc("GET /devcards/{project}/{devcard}", s.handleDevcard)
	mux.HandleFunc("GET /devcards/{project}/{devcard}/edit", s.handleEdit)
	mux.HandleFunc("POST /devcards/sse", s.handleSSE)

	mux.HandleFunc("GET /devcards/css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		w.Write([]byte(s.cfg.CSS()))
	})
	mux.HandleFunc("GET /file", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		http.ServeFile(w, r, path)
	})
	mux.HandleFunc("GET /devcards/favicon.png", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, assetsFS, "/assets/favicon.png")
	})
	mux.HandleFunc("GET /devcards/datastar.js", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, assetsFS, "/assets/datastar.js")
	})

	mux.HandleFunc("POST /devcards/init-config", func(w http.ResponseWriter, r *http.Request) {
		sse := datastar.NewSSE(w, r)
		err := s.cfg.Create()
		if err != nil {
			sse.MergeSignals([]byte(`{initConfigError: '` + err.Error() + `'}`))
			return
		}
		s.restart()
		mergeRefresh(sse)
	})
	mux.HandleFunc("POST /devcards/restart", func(w http.ResponseWriter, r *http.Request) {
		s.restart()
		mergeRefresh(datastar.NewSSE(w, r))
	})

	// TODO: remove?
	mux.HandleFunc("/exit", func(http.ResponseWriter, *http.Request) {
		fmt.Println("[server] shutting down the server")
		s.Shutdown()
		time.Sleep(time.Second)
		os.Exit(0)
	})
	mux.HandleFunc("/emergency-exit", func(http.ResponseWriter, *http.Request) {
		fmt.Println("goodbye world")
		os.Exit(1)
	})
}

func mergeSignalsf(sse *datastar.ServerSentEventGenerator, format string, args ...any) error {
	data := []byte(fmt.Sprintf(format, args...))
	return sse.MergeSignals(data)
}

func mergeRefresh(sse *datastar.ServerSentEventGenerator) {
	sse.MergeFragments(`
	<div id="refresh">
		<div data-on-load="window.location.href=window.location.href"></div>
	</div>`)
}

func (s *server) handleHomePage(w http.ResponseWriter, r *http.Request) {
	homePage(s.cfg).Render(r.Context(), w)
}

func (s *server) handleProject(w http.ResponseWriter, r *http.Request) {
	projectName := r.PathValue("project")

	project := s.projects[projectName]
	if project == nil {
		log.Println("no such project: " + projectName)
		return
	}

	fromDevcard := r.URL.Query().Get("from")
	projectPage(projectName, fromDevcard, project.GetDevcards()).Render(r.Context(), w)
}

type navBar struct {
	prev, pkg, next string
}

func makeNavBar(cardsMeta project.DevcardsMetaSlice, devcardName string) navBar {
	m := cardsMeta.Lookup(devcardName)
	navBar := navBar{pkg: m.Package}
	cardsMeta = cardsMeta.FilterByImportPath(m.ImportPath)
	if len(cardsMeta) > 1 {
		prev, next := len(cardsMeta)-1, 0
		for i, meta := range cardsMeta {
			if meta.Name == devcardName && i > 0 {
				prev = i - 1
			}
			if meta.Name == devcardName && i < len(cardsMeta)-1 {
				next = i + 1
			}
		}
		navBar.prev = cardsMeta[prev].Name
		navBar.next = cardsMeta[next].Name
	}
	return navBar
}

func (s *server) handleDevcard(w http.ResponseWriter, r *http.Request) {
	projectName := r.PathValue("project")
	devcardName := r.PathValue("devcard")

	project := s.projects[projectName]
	if project == nil {
		log.Println("no such project: " + projectName)
		return
	}

	navBar := makeNavBar(project.GetDevcards(), devcardName)
	err := devcardPage(s.cfg, s.cfg.CSS(), projectName, devcardName, devcardName, navBar).Render(r.Context(), w)
	if err != nil {
		log.Println("handleDevcard error: " + err.Error())
	}
}

func (s *server) findRunner(projectName string, runnerId string) chan any {
	project := s.projects[projectName]
	if project == nil {
		return nil
	}
	return project.GetRunner(runnerId)
}

func (s *server) stopRunner(projectName string, runnerId string) {
	if project := s.projects[projectName]; project != nil {
		project.StopRunner(runnerId)
	}
}

func (s *server) handleSSE(w http.ResponseWriter, r *http.Request) {
	var x struct {
		Devcards struct{ Project, Name, RunnerId string }
	}
	json.NewDecoder(r.Body).Decode(&x)

	sse := datastar.NewSSE(w, r)
	cells := map[string]bool{}

	runnerId := x.Devcards.RunnerId
	ch := s.findRunner(x.Devcards.Project, runnerId)
	if ch == nil {
		project := s.projects[x.Devcards.Project]
		if project == nil {
			log.Println("no such project: " + x.Devcards.Project)
			return
		}
		runnerId = project.StartRunner(x.Devcards.Name)
		ch = s.findRunner(x.Devcards.Project, runnerId)
		mergeSignalsf(sse, `{devcards: {runnerId:'%s'}}`, runnerId)
	}

	var initStdout, initStderr bool
	for msg := range ch {
		// log.Printf("[server] msg %T %#v\n", msg, msg)
		var err error
		switch x := msg.(type) {
		case runner.Meta:
			if x.BuildTime != "" {
				mergeSignalsf(sse, `{devcards: {buildTime:'%s'}}`, x.BuildTime)
			}
			if x.RunTime != "" {
				mergeSignalsf(sse, `{devcards: {runTime:'%s'}}`, x.RunTime)
			}

		case runner.Title:
			err = sse.MergeFragmentf(`<title id="-dc-tab-title">%s</title>`, x.Title)
			if err == nil {
				var buf bytes.Buffer
				dcTitle(x.Title, s.cfg.Editor != "").Render(r.Context(), &buf)
				err = sse.MergeFragments(buf.String())
			}

		case runner.CSS:
			err = sse.MergeFragmentf(`<style id="-dc-style">%s</style>`, x.Stylesheet)

		case runner.Error:
			var buf bytes.Buffer
			dcError(x).Render(r.Context(), &buf)
			err = sse.MergeFragments(buf.String())

		case runner.Stdout:
			if !initStdout {
				initStdout = true
				var buf bytes.Buffer
				dcStdout("").Render(r.Context(), &buf)
				sse.MergeFragments(buf.String())
			}
			line := "<span>" + x.Line + "</span>"
			err = sse.MergeFragments(line,
				datastar.WithMergeAppend(),
				datastar.WithSelector("#-dc-stdout"))

		case runner.Stderr:
			if !initStderr {
				initStderr = true
				var buf bytes.Buffer
				dcStderr("").Render(r.Context(), &buf)
				sse.MergeFragments(buf.String())
			}
			line := "<span>" + x.Line + "</span>"
			err = sse.MergeFragments(line,
				datastar.WithMergeAppend(),
				datastar.WithSelector("#-dc-stderr"))

		case runner.Cell:
			if !cells[x.Id] {
				cells[x.Id] = true
				err = sse.MergeFragments(
					fmt.Sprintf(`<div id="%s"/>`, x.Id),
					datastar.WithMergeAppend(),
					datastar.WithSelector("#-dc-cells"))
			}
			if err == nil {
				err = sse.MergeFragmentf(`<div class="-dc-cell" id="%s">%s</div>`, x.Id, x.Content)
			}

		case runner.Card:
			initStdout, initStderr = false, false
			cells = map[string]bool{}

			var cellsStrs []string
			for _, cell := range x.Cells {
				cellsStrs = append(cellsStrs, fmt.Sprintf(`<div class="-dc-cell" id="%s">%s</div>`, cell.Id, cell.Content))
				cells[cell.Id] = true
			}

			var stdout, stderr string
			if x.Stdout != "" {
				initStdout = true
				var buf bytes.Buffer
				dcStdout(x.Stdout).Render(r.Context(), &buf)
				stdout = buf.String()
			} else {
				initStdout = false
				stdout = `<div id="-dc-stdout-box"></div>`
			}
			if x.Stderr != "" {
				initStderr = true
				var buf bytes.Buffer
				dcStderr(x.Stderr).Render(r.Context(), &buf)
				stderr = buf.String()
			} else {
				initStderr = false
				stderr = `<div id="-dc-stderr-box"></div>`
			}

			sse.MergeFragmentf(`<div id="-dc-cells">%s</div>%s%s`,
				strings.Join(cellsStrs, ""), stdout, stderr)

			var buf bytes.Buffer
			dcError(runner.Error{}).Render(r.Context(), &buf)
			err = sse.MergeFragments(buf.String())

		case runner.Heartbeat:
			err = sse.MergeFragments("")

		default:
			t := fmt.Sprintf("%T", msg)
			fmt.Println("[server]", "unknown message", t, ">", msg)
		}

		if err != nil {
			break
		}
	}

	s.stopRunner(x.Devcards.Project, runnerId)
	sse.MergeSignals([]byte(`{devcards: {disconnected: true}}`))
}
