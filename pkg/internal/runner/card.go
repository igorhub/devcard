package runner

type UpdateMessage interface{ updateMessage() }

type Card struct {
	Cells  []Cell
	Stdout string
	Stderr string

	ids map[string]bool
}

func newCard() *Card {
	return &Card{ids: map[string]bool{}}
}

func (c *Card) addCell(cell Cell) {
	if c.ids[cell.Id] {
		return
	}
	c.Cells = append(c.Cells, cell)
	c.ids[cell.Id] = true
}

type Cell struct {
	Id      string
	Content string
}

type Meta struct {
	BuildTime string
	RunTime   string
}

type Error struct {
	Title string
	Err   error
}

type Title struct {
	Title string
}

type CSS struct {
	Values     []string
	Stylesheet string
}

type Stdout struct {
	Line string
}

type Stderr struct {
	Line string
}

type Heartbeat struct{}

func (Card) updateMessage()      {}
func (Cell) updateMessage()      {}
func (Meta) updateMessage()      {}
func (Error) updateMessage()     {}
func (Title) updateMessage()     {}
func (CSS) updateMessage()       {}
func (Stderr) updateMessage()    {}
func (Stdout) updateMessage()    {}
func (Heartbeat) updateMessage() {}
