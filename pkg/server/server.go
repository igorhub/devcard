package server

import (
	"fmt"
	"net/http"
	"slices"

	"github.com/igorhub/devcard/pkg/internal/project"
)

type server struct {
	cfg     Config
	clients []*client
	handler http.Handler

	events   chan serverMessage
	projects map[string]*project.Project
	restart  chan<- struct{}
}

func NewServer(cfg Config, restart chan<- struct{}) *server {
	s := &server{
		cfg:      cfg,
		events:   make(chan serverMessage, 256),
		projects: make(map[string]*project.Project),
		restart:  restart,
	}

	for _, cfgProject := range cfg.Projects {
		p := project.NewProject(cfgProject.Name, cfgProject.Dir)
		s.projects[cfgProject.Name] = p
		go func() {
			for range p.Update {
				s.events <- msgRefreshClients{p}
			}
		}()
	}

	go s.manageClients()

	mux := http.NewServeMux()
	s.addRoutes(cfg, mux)
	s.handler = mux
	return s
}

func (s *server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s.handler.ServeHTTP(w, req)
}

func (s *server) Shutdown() {
	done := make(chan struct{})
	s.events <- msgShutdown{done}
	<-done
}

type serverMessage interface{ serverMessage() }

type msgRegisterClient struct {
	client *client
}

type msgUnregisterClient struct {
	client *client
}

type msgUnblockClient struct {
	blockId string
}

type msgRefreshClients struct {
	project *project.Project
}

type msgShutdown struct {
	done chan struct{}
}

func (msgRegisterClient) serverMessage()   {}
func (msgUnregisterClient) serverMessage() {}
func (msgUnblockClient) serverMessage()    {}
func (msgRefreshClients) serverMessage()   {}
func (msgShutdown) serverMessage()         {}

func (s *server) manageClients() {
	for event := range s.events {
		// log.Printf("manageClients: %#v", event)
		switch e := event.(type) {
		case msgRegisterClient:
			s.clients = append(s.clients, e.client)

		case msgUnregisterClient:
			e.client.project.RemoveRepo(e.client.repo)
			e.client.close()
			s.clients = slices.DeleteFunc(s.clients, func(c *client) bool { return c == e.client })

		case msgUnblockClient:
			for _, c := range s.clients {
				if c.unblock == e.blockId {
					close(c.unblockC)
					c.unblockC = nil
					c.unblock = ""
				}
			}

		case msgRefreshClients:
			for _, c := range s.clients {
				if c.project == e.project {
					c.refresh()
				}
			}

		case msgShutdown:
			for _, c := range s.clients {
				c.close()
			}
			for _, p := range s.projects {
				p.Shutdown()
			}
			close(e.done)
			return

		default:
			panic(fmt.Errorf("manage clients: unknown event %T", e))
		}
	}
}
