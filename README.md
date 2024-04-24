# Devcards

Devcards provides interactive visual environment for Go,
in a way that's similar to REPL and computational notebooks such as Jupyter.


# How it works

A devcard is a playground for visualizations and quick experiments.
You write its code with our own editor in you own codebase,
and the devcards web app turns it into a web page that's shown in your browser.
The app watches your project's source files and re-renders the page on each save.

See this [short video](https://youtu.be/RciKxTDfEUA) for a quick demonstration.


# Getting started

Perhaps the easiest place to start is to the [devcard examples repo](https://github.com/igorhub/devcard-examples).

If you went through the examples already
(or have no patience for toy code)
follow the following instructions.

Install the devcards web app:
	go install github.com/igorhub/devcard/cmd/devcards@latest

Add devcard dependency to your Go modules (I recommend making a separate branch for it):
	go get github.com/igorhub/devcard

Start devcards from your project's directory (alternatively, add your project to the config file):
	cd /path/to/your/project
	devcards


Write your first devcard:
```go
package yourpackage

import "github.com/igorhub/devcard"

func DevcardFoobar(dc *devcard.Devcard) {
    dc.SetTitle("Untitled")

    dc.Md("This is a new devcard...")
}
```


# Documentation

For introduction into devcards, see [devcard examples pages](https://igorhub.github.io/devcard-examples/DevcardAnatomy.html).

For API reference, see https://godocs.io/github.com/igorhub/devcard.


# Troubleshooting

Devcards is a young project.
I've done reasonable job ironing out the bugs, but I expect some to still lurk beneath the surface.
Most of the time refreshing the page will fix everything,
but please let me know about errors you're able to reproduce.
I'll appreciate your help.
This will make the project better.


# Acknowledgements

* Devcards owes its name and primary idea to Bruce Hauman's [devcards](https://github.com/bhauman/devcards),
although it's more bare-bones and limited in scope.

* Devcards' builtin CSS style is based upon the excellent [new.css](https://github.com/xz/new.css).

* Ace of Spades icon is designed by [DesignContest](http://www.designcontest.com/) / [CC BY](http://creativecommons.org/licenses/by/4.0/).
