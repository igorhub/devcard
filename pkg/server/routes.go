package server

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"math/rand"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/gorilla/websocket"
)

func (s *server) addRoutes(cfg Config, mux *http.ServeMux) {
	mux.HandleFunc("GET /", s.handleProjects)
	mux.HandleFunc("GET /dc/{project}", s.handleProjectDevcards)
	mux.HandleFunc("GET /dc/{project}/{devcard}", s.handleDevcard)
	mux.HandleFunc("GET /ws", s.handleWS)
	mux.HandleFunc("GET /file", s.handleFile)
	mux.HandleFunc("GET /unblock/{id}", s.handleUnblock)
	mux.HandleFunc("GET /open/{project}/{devcard}", s.handleEdit)
	mux.HandleFunc("GET /restart", s.handleRestart)
	mux.HandleFunc("GET /init-config", s.handleInitConfig)

	mux.HandleFunc("GET /favicon.png", handleFavicon)
	mux.HandleFunc("GET /stylesheet/css", cssHandler(cfg))
}

func (s *server) handleProjects(w http.ResponseWriter, req *http.Request) {
	b := new(bytes.Buffer)
	fmt.Fprintf(b, "#### Projects\n\n")
	for _, project := range s.cfg.Projects {
		fmt.Fprintf(b, "* [%s](%s): %s\n", project.Name, "/dc/"+project.Name, project.Dir)
	}

	fmt.Fprintf(b, "\n#### Config\n\n")
	switch {
	case s.cfg.Err != nil && errors.Is(s.cfg.Err, fs.ErrNotExist):
		fmt.Fprintf(b, "Config file doesn't exist at `%s`\n\n", s.cfg.Path)
		fmt.Fprintf(b, `<form action="/init-config"><input type="submit" value="Create initial config" /></form>`)
	case s.cfg.Err != nil:
		fmt.Fprintf(b, "Unable to load the config: `%s`\n\n", s.cfg.Err)
		if s.cfg.Data != nil {
			fmt.Fprintf(b, "Content:\n")
			fmt.Fprintf(b, "```\n%s\n```\n", string(s.cfg.Data))
		}
	default:
		fmt.Fprintf(b, "Location: `%s`\n\n", s.cfg.Path)
		fmt.Fprintf(b, "Content:\n")
		fmt.Fprintf(b, "```\n%s\n```\n", string(s.cfg.Data))
	}

	fmt.Fprintf(b, "\n#### Server\n\n")
	fmt.Fprintf(b, `<form action="/restart"><input type="submit" value="Restart the server" /></form>`)

	w.Header().Set("Content-Type", "text/html")
	w.Write(page{
		title: "Devcards",
		body:  MdToHTML(b.String()),
	}.generate())
}

func (s *server) handleInitConfig(w http.ResponseWriter, req *http.Request) {
	err := s.cfg.Create()
	if err != nil {
		w.Write(page{
			title: "Devcards",
			body:  "failed to create config: " + err.Error(),
		}.generate())
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		s.handleRestart(w, req)
	}
}

func (s *server) handleRestart(w http.ResponseWriter, req *http.Request) {
	redirect := req.URL.Query().Get("redirect")
	if redirect == "" {
		redirect = "/"
	}

	html := page{
		title: "Devcards",
		body: `Server is being restarted...
        <script type="text/javascript">
            setTimeout("window.location = \"{{redirect}}\"", 1000);
        </script>`,
	}.generate()
	html = bytes.ReplaceAll(html, []byte("{{redirect}}"), []byte(redirect))
	w.Write(html)

	go func() {
		time.Sleep(50 * time.Millisecond)
		close(s.restart)
	}()
}

func handleFavicon(w http.ResponseWriter, req *http.Request) {
	http.ServeFile(w, req, "/storage/down/Designcontest-Casino-Ace-of-Spades.96.png")
}

//go:embed assets/new.css
var newCSS []byte

//go:embed assets/light.css
var lightTheme []byte

//go:embed assets/dark.css
var darkTheme []byte

//go:embed assets/gruvbox-light.css
var gruvboxLightTheme []byte

//go:embed assets/gruvbox-dark.css
var gruvboxDarkTheme []byte

func cssHandler(cfg Config) http.HandlerFunc {
	highlighterCSS := newHighlighter(cfg.Appearance.CodeHighlighting).CSS()
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		for _, stylesheet := range cfg.Appearance.Stylesheets {
			switch stylesheet {
			case "builtin":
				w.Write(newCSS)
			case "builtin/light", "":
				w.Write(lightTheme)
			case "builtin/dark":
				w.Write(darkTheme)
			case "builtin/gruvbox-light":
				w.Write(gruvboxLightTheme)
			case "builtin/gruvbox-dark":
				w.Write(gruvboxDarkTheme)
			default:
				data, err := os.ReadFile(stylesheet)
				if err != nil {
					log.Println("Can't read CSS file:", err)
					break
				}
				w.Write(data)
			}
		}
		w.Write(highlighterCSS)
	}
}

func newClientId() string {
	return fmt.Sprintf("cl-%d-%d", time.Now().UnixMicro(), rand.Uint32())
}

func (s *server) handleDevcard(w http.ResponseWriter, req *http.Request) {
	projectName := req.PathValue("project")
	devcardName := req.PathValue("devcard")
	w.Header().Set("Content-Type", "text/html")
	w.Write(page{
		title:       s.projects[projectName].DevcardInfo(devcardName).Caption(),
		clientId:    newClientId(),
		clientKind:  ClientDevcard,
		url:         req.URL.String(),
		projectName: projectName,
		devcardName: devcardName,
	}.generate())
}

func (s *server) handleProjectDevcards(w http.ResponseWriter, req *http.Request) {
	projectName := req.PathValue("project")
	w.Header().Set("Content-Type", "text/html")
	w.Write(page{
		title:       "Devcards: " + projectName,
		clientId:    newClientId(),
		clientKind:  ClientListDevcards,
		url:         req.URL.String(),
		projectName: projectName,
	}.generate())
}

const (
	ClientDevcard      = "ClientDevcard"
	ClientListDevcards = "ClientList"
)

func (s *server) createClient(kind, clientId, url, projectName, devcardName string) *client {
	c := newClient(s.cfg, clientId, url, s.projects[projectName])
	unregister := func() { s.events <- msgUnregisterClient{client: c} }
	switch {
	case s.projects[projectName] == nil:
		c.initError(fmt.Errorf("project %q doesn't exist", projectName), unregister)
	case kind == ClientDevcard:
		c.initDevcard(devcardName, unregister)
	case kind == ClientListDevcards:
		c.initListDevcards(unregister)
	default:
		c.initError(fmt.Errorf("unknown client kind %q", kind), unregister)
	}
	s.events <- msgRegisterClient{client: c}
	return c
}

var websocketUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func (s *server) handleWS(w http.ResponseWriter, req *http.Request) {
	conn, err := websocketUpgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Println(err)
		return
	}

	var c *client
	q := req.URL.Query()
	clientId := q.Get("clientId")
	i := slices.IndexFunc(s.clients, func(c *client) bool { return c.id == clientId })
	if i == -1 {
		log.Printf("Create client %q", clientId)
		c = s.createClient(q.Get("clientKind"), clientId, q.Get("url"), q.Get("projectName"), q.Get("devcardName"))
	} else {
		c = s.clients[i]
	}

	c.connect(conn)
	c.refresh()
}

func (s *server) handleFile(w http.ResponseWriter, req *http.Request) {
	path := req.URL.Query().Get("path")
	http.ServeFile(w, req, path)
}

func (s *server) handleUnblock(w http.ResponseWriter, req *http.Request) {
	s.events <- msgUnblockClient{req.PathValue("id")}
}
