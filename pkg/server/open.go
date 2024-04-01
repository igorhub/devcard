package server

import (
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/igorhub/devcard"
)

func (s *server) handleEdit(w http.ResponseWriter, req *http.Request) {
	projectName := req.PathValue("project")
	devcardName := req.PathValue("devcard")

	errorHeader := []byte("Unable to open devcard for editing\n\n")

	project := s.projects[projectName]
	if project == nil {
		w.Write(errorHeader)
		w.Write([]byte("Project " + projectName + " not found."))
		return
	}

	info := project.DevcardInfo(devcardName)
	if info == (devcard.DevcardInfo{}) {
		w.Write(errorHeader)
		w.Write([]byte("Devcard " + devcardName + " not found in " + projectName + "."))
		return
	}

	var err error
	switch {
	case s.cfg.Opener != "":
		err = openCustom(s.cfg.Opener, filepath.Join(project.Dir, info.Path), info.Line)
	case strings.ToLower(s.cfg.Editor) == "emacs":
		err = openInEmacs(filepath.Join(project.Dir, info.Path), info.Line)
	case strings.ToLower(s.cfg.Editor) == "vscode":
		err = openInVscode(filepath.Join(project.Dir, info.Path), info.Line)
	}

	if err != nil {
		w.Write(errorHeader)
		w.Write([]byte(err.Error()))
	}
}

func openInEmacs(path string, line int) error {
	cmd := fmt.Sprintf(`(progn
(find-file "%s")
(goto-line %d)
(recenter-top-bottom 5))`, path, line)
	return exec.Command("emacsclient", "--eval", cmd).Run()
}

func openInVscode(path string, line int) error {
	return exec.Command("code", "-g", path+":"+strconv.Itoa(line)).Run()
}

func openCustom(opener, path string, line int) error {
	return exec.Command(opener, path, strconv.Itoa(line)).Run()
}
