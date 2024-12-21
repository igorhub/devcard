package devcard

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime/debug"
	"strings"
)

// DevcardProducer is a function that fills an empty devcard with content.
type DevcardProducer func(*Devcard)

func produce(tcpAddress, tempDir string, producer DevcardProducer) (dc *Devcard) {
	dc = newDevcard("Untitled devcard", tempDir)
	current = dc

	done := make(chan struct{})
	if tcpAddress != "" {
		err := createTCPClient(tcpAddress, dc.control, dc.updates, done)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
	} else {
		createEchoClient(dc.updates, done)
	}

	defer func() {
		e := recover()
		if e == interrupt {
			dc.Error("", "interrupted")
		} else if e != nil {
			dc.Jump()
			dc.Error("Panic!")
			switch x := e.(type) {
			case error:
				dc.Append("// " + x.Error())
				dc.Append(pprint(x) + "\n")
			case string:
				dc.Append(x)
			}
			dc.Append("\n" + string(debug.Stack()))
		}
		// Close dc.updates and wait for the TCP client to write all its messages.
		close(dc.updates)
		<-done
	}()

	producer(dc)
	return
}

func createTCPClient(address string, control chan<- string, updates <-chan string, done chan struct{}) error {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return fmt.Errorf("unable to create TCP client: %w", err)
	}

	go func() {
		r := bufio.NewReader(conn)
		for {
			s, err := r.ReadString('\n')
			if err != nil && errors.Is(err, net.ErrClosed) {
				return
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading from TCP connection: %#v", err)
				return
			}

			s = strings.TrimSpace(s)
			parts := strings.Split(s, " ")
			switch parts[0] {
			case "exit":
				conn.Close()
				os.Exit(0)
			case "unblock":
				control <- parts[1]
			default:
				fmt.Fprintf(os.Stderr, "Malformed message on TCP connection: %#v", s)
			}
		}
	}()

	go func() {
		for s := range updates {
			_, err := conn.Write([]byte(s + "\n"))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to write to TCP connection: %s\nMessage: %s", err, s)
				break
			}
		}
		conn.Close()
		close(done)
	}()

	return nil
}

func createEchoClient(updates <-chan string, done chan struct{}) {
	go func() {
		for s := range updates {
			fmt.Println(s)
		}
		close(done)
	}()
}

var interrupt = errors.New("devcard: interrupt")

func Interrupt() {
	panic(interrupt)
}
