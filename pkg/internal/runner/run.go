package runner

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
	"sync"

	"github.com/igorhub/devcard"
	"github.com/igorhub/devcard/pkg/internal/file"
)

const (
	PipeStdout = "Stdout"
	PipeStderr = "Stderr"
)

// Run uses "go run" to produce and return a devcard.
//
// If errors occur, they're written into devcard.Error field of the devcard.
func (r *Runner) run(ctx context.Context, updates chan<- any) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		updates <- evBuilt{}
		updates <- Error{Title: "Failed to create TCP listener", Err: err}
		return
	}
	defer listener.Close()

	wg := sync.WaitGroup{}
	go func() {
		conn, err := listener.Accept()
		if err != nil && errors.Is(err, net.ErrClosed) {
			return
		} else if err != nil {
			updates <- evBuilt{}
			updates <- Error{Title: "Failed to accept TCP connection from the devcard", Err: err}
			cancel()
			return
		}
		defer conn.Close()
		updates <- evBuilt{}

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
					updates <- Error{Title: "Failed to read from the devcard's TCP connection", Err: err}
					cancel()
					return
				}
				updates <- unmarshalDevcardMessage(s)
			}
		}()

		<-ctx.Done()
		if _, err := conn.Write([]byte("exit\n")); err != nil {
			log.Println("Error writing \"exit\" to conn:", err)
		}
		conn.Close()
	}()

	cmd := exec.CommandContext(ctx, "go", "run", "-tags", "devcard", ".", r.dir, r.transientDir, r.cardMeta.Name, listener.Addr().String())
	cmd.Dir = filepath.Join(r.dir, file.DevcardMainDir(r.cardMeta))

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		updates <- evBuilt{}
		updates <- Error{Title: "Failed to create stdout pipe", Err: err}
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		updates <- evBuilt{}
		updates <- Error{Title: "Failed to create stderr pipe", Err: err}
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
		err := fmt.Errorf("go run: %w", err)
		updates <- Error{Title: "Execution failure", Err: err}
	}

	wg.Wait()
}

const maxPipeLines = 10000

func readFromPipe(pipe io.Reader, pipeName string) <-chan UpdateMessage {
	msg := func(line string) UpdateMessage {
		switch pipeName {
		case PipeStdout:
			return Stdout{line}
		case PipeStderr:
			return Stderr{line}
		default:
			panic("incorrect pipe name: " + pipeName)
		}
	}

	updates := make(chan UpdateMessage)
	go func() {
		defer close(updates)
		var initialized bool
		r := bufio.NewReader(pipe)
		n := maxPipeLines
		for {
			line, err := r.ReadString('\n')
			if err == io.EOF || errors.Is(err, fs.ErrClosed) {
				break
			}
			if err != nil {
				updates <- Error{Title: "Failed to read from devcard's " + pipeName, Err: err}
				break
			}
			if !initialized {
				updates <- evBuilt{}
				initialized = true
			}
			n--
			if n <= 0 {
				if n == 0 {
					updates <- msg("\n... output limit exceeded")
				}
				continue
			}
			updates <- msg(line)
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

		// Other types
		Title string
		CSS   []string `json:"css"`
	}{}

	err := json.Unmarshal([]byte(msg), &x)
	if err != nil {
		err = fmt.Errorf("error: %s\n\nmessage: %s", err, msg)
		return Error{Title: "Failed to decode message from the devcard", Err: err}
	}

	switch x.MsgType {
	case devcard.MessageTypeCell:
		cell, err := devcard.UnmarshalCell(x.CellType, x.Cell)
		if err != nil {
			err = fmt.Errorf("error: %s\n\ncell type: %s\n\ncell: %s", err, x.CellType, string(x.Cell))
			return Error{Title: "Failed to decode a cell from the devcard", Err: err}
		}
		return evCell{Id: x.Id, Cell: cell}

	case devcard.MessageTypeTitle:
		if x.Title != "" {
			return Title{x.Title}
		}
		return Error{
			Title: "Internal error",
			Err:   fmt.Errorf("incorrect title message: %#v", x),
		}

	case devcard.MessageTypeCSS:
		return CSS{Values: x.CSS}

	case devcard.MessageTypeError:
		return Error{
			Title: "Internal error",
			Err:   fmt.Errorf("message type %q is not supported", x.MsgType),
		}

	default:
		return Error{
			Title: "Internal error: malformed message from the devcard",
			Err:   fmt.Errorf("%#v", x),
		}
	}
}
