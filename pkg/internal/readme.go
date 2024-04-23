package yourpackage

import (
	"bufio"
	"os"
	"strings"

	"github.com/igorhub/devcard"
)

func Devcard_Readme(c *devcard.Devcard) {
	c.SetTitle("README.md")

	f, err := os.Open("README.md")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if line := scanner.Text(); !strings.HasPrefix(line, "//") {
			lines = append(lines, line)
		}
	}
	c.Md(strings.Join(lines, "\n"))
}
