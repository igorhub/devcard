package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	_ "unsafe"

	"github.com/igorhub/devcard"
)

// write marshals the devcard to JSON and writes it to outFile, or to standard
// output (if outFile is "-").
//
// If an error occurs, it writes the error message into standard error.
func write(outFile string, devcard *devcard.Devcard) {
	data, err := json.MarshalIndent(devcard, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't marshal devcard: %s", err)
	}

	if outFile == "-" {
		fmt.Println(string(data))
	} else {
		err := os.WriteFile(outFile, data, 0666)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Can't write output for the devcard.\nReason: %s", err)
		}
	}
}

// writeStubDevcard writes a stub devcard to outFile.
// If the process exits while building a devcard, this stub will be transmitted down the line.
// Otherwise, outFile will be overwritten with the real devcard data.
func writeStubDevcard(outFile string, card string) {
	if outFile == "-" {
		return
	}
	dc := &devcard.Devcard{Title: card}
	dc.Error("Process exited prematurely")
	write(outFile, dc)
}

func functionName(fn interface{}) string {
	fullName := runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
	i := strings.LastIndex(fullName, ".")
	return fullName[i+1:]
}

// ProduceDevcard creates an empty devcard, fills it with content by running the producer function,
// marshals it to JSON, and writes to outFile.
func ProduceDevcardWithTCP(address, tempDir string, producer devcard.DevcardProducer) {
	produce(address, tempDir, producer)
}

// ProduceDevcard creates an empty devcard, fills it with content by running the producer function,
// marshals it to JSON, and writes to outFile.
func ProduceDevcardWithJSON(tempDir string, producer devcard.DevcardProducer) {
	outFile := filepath.Join(tempDir, "devcard.json")
	writeStubDevcard(outFile, functionName(producer))
	dc := produce("", tempDir, producer)
	write(outFile, dc)
}

//go:linkname produce github.com/igorhub/devcard.produce
func produce(netAddress, tempDir string, producer devcard.DevcardProducer) *devcard.Devcard
