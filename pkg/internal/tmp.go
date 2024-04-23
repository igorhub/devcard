// TODO: See bad error message when package names differ
package yourpackage

import "github.com/igorhub/devcard"

func DevcardFoobar(dc *devcard.Devcard) {
	dc.SetTitle("Untitled")

	dc.Md("This is a new devcard...")
}
