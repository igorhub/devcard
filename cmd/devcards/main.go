package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/igorhub/devcard/pkg/server"
)

const version = "v0.11.0"

func main() {
	var port int
	var showVersion bool
	flag.IntVar(&port, "port", 0, "Port for the devcards server")
	flag.BoolVar(&showVersion, "version", false, "Show version")
	flag.Parse()

	if showVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	server.Run(port)
}
