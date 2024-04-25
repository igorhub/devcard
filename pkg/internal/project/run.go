package project

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/igorhub/devcard"
)

type UpdateMessage interface{ updateMessage() }

type MsgReady struct{}

type MsgError struct {
	Title string
	Err   error
}

type MsgInfo struct {
	Title string
}

type MsgCell struct {
	Id   string
	Cell devcard.Cell
}

const (
	PipeStdout = "Stdout"
	PipeStderr = "Stderr"
)

type MsgPipeOut struct {
	Pipe string
	Line string
}

func (MsgReady) updateMessage()   {}
func (MsgError) updateMessage()   {}
func (MsgInfo) updateMessage()    {}
func (MsgCell) updateMessage()    {}
func (MsgPipeOut) updateMessage() {}

// Run uses "go run" to produce and return a devcard.
//
// If errors occur, they're written into devcard.Error field of the devcard.
func (r *Repo) Run(ctx context.Context, control <-chan string, updates chan<- UpdateMessage) {
	r.runLock.Lock()
	defer r.runLock.Unlock()
	defer close(updates)

	err := r.Prepare()
	if err != nil {
		updates <- MsgReady{}
		updates <- MsgError{Title: "Failed to prepare the repo", Err: err}
		return
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		updates <- MsgReady{}
		updates <- MsgError{Title: "Failed to create TCP listener", Err: err}
		return
	}
	defer listener.Close()

	wg := sync.WaitGroup{}
	go func() {
		conn, err := listener.Accept()
		if err != nil && errors.Is(err, net.ErrClosed) {
			return
		} else if err != nil {
			updates <- MsgReady{}
			updates <- MsgError{Title: "Failed to accept TCP connection from the devcard", Err: err}
			cancel()
			return
		}
		defer conn.Close()
		updates <- MsgReady{}

		go func() {
			wg.Add(1)
			defer wg.Done()
			r := bufio.NewReader(conn)
			for {
				s, err := r.ReadString('\n')
				if err != nil && errors.Is(err, io.EOF) {
					return
				} else if err != nil {
					log.Printf("Failed to read from the devcard's TCP connection: %s\n%#v", err, err)
					updates <- MsgError{Title: "Failed to read from the devcard's TCP connection", Err: err}
					cancel()
					return
				}
				updates <- unmarshalDevcardMessage(s)
			}
		}()

		for {
			select {
			case <-ctx.Done():
				_, err := conn.Write([]byte("exit\n"))
				if err != nil {
					log.Println("Error writing \"exit\" to conn:", err)
				}
				conn.Close()
				return
			case s, ok := <-control:
				if ok {
					_, err := conn.Write([]byte(s + "\n"))
					if err != nil {
						log.Printf("Error writing %q to conn: %s", s, err)
						cancel()
						return
					}
				}
			}
		}
	}()

	mainFile := filepath.Join(FindMainDir(r.DevcardInfo), generatedMainFile)
	cmd := exec.CommandContext(ctx, "go", "run", mainFile, listener.Addr().String(), r.TransientDir)
	cmd.Dir = r.Dir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		updates <- MsgReady{}
		updates <- MsgError{Title: "Failed to create stdout pipe", Err: err}
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		updates <- MsgReady{}
		updates <- MsgError{Title: "Failed to create stderr pipe", Err: err}
		return
	}

	stderrC := readFromPipe(stderr, PipeStderr)
	stdoutC := readFromPipe(stdout, PipeStdout)
	wg.Add(2)
	go func() {
		for msg := range stdoutC {
			updates <- msg
		}
		wg.Done()
	}()
	go func() {
		for msg := range stderrC {
			updates <- msg
		}
		wg.Done()
	}()

	err = cmd.Run()
	if err != nil {
		updates <- MsgError{Title: "Execution failure", Err: err}
	}

	wg.Wait()
}

func readFromPipe(pipe io.Reader, pipeName string) <-chan UpdateMessage {
	updates := make(chan UpdateMessage)
	go func() {
		defer close(updates)
		r := bufio.NewReader(pipe)
		for {
			line, err := r.ReadString('\n')
			if err == io.EOF || errors.Is(err, fs.ErrClosed) {
				break
			}
			if err != nil {
				updates <- MsgError{Title: "Failed to read from devcard's " + pipeName, Err: err}
				break
			}
			updates <- MsgPipeOut{Pipe: pipeName, Line: line}
		}
	}()
	return updates
}

func unmarshalDevcardMessage(msg string) UpdateMessage {
	x := struct {
		MsgType string `json:"msg_type"`

		// Cell type
		Id       string
		CellType string `json:"cell_type"`
		Cell     json.RawMessage

		// Info type
		Title string
	}{}

	err := json.Unmarshal([]byte(msg), &x)
	if err != nil {
		err = fmt.Errorf("error: %s\n\nmessage: %s", err, msg)
		return MsgError{Title: "Failed to decode message from the devcard", Err: err}
	}

	switch x.MsgType {
	case devcard.MessageTypeCell:
		cell, err := devcard.UnmarshalCell(x.CellType, x.Cell)
		if err != nil {
			err = fmt.Errorf("error: %s\n\ncell type: %s\n\ncell: %s", err, x.CellType, string(x.Cell))
			return MsgError{Title: "Failed to decode a cell from the devcard", Err: err}
		}
		return MsgCell{Id: x.Id, Cell: cell}

	case devcard.MessageTypeInfo:
		return MsgInfo{
			Title: x.Title,
		}

	case devcard.MessageTypeError:
		return MsgError{
			Title: "Internal error",
			Err:   fmt.Errorf("message type %q is not supported", x.MsgType),
		}

	default:
		return MsgError{
			Title: "Internal error: malformed message from the devcard",
			Err:   fmt.Errorf("message type %q is not supported", x.MsgType),
		}
	}
}

func (r *Repo) Test(ctx context.Context) int {
	r.runLock.Lock()
	defer r.runLock.Unlock()

	cmd := exec.CommandContext(ctx, "go", "test", r.DevcardInfo.ImportPath)
	cmd.Dir = r.Dir
	b, err := cmd.CombinedOutput()
	if err == nil {
		return 0
	}

	failedTests := 0
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "--- FAIL:") {
			failedTests++
		}
	}

	return failedTests
}
