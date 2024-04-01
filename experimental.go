package devcard

import "fmt"

// devcard.go

// Not documented. Subject to change.
func (d *Devcard) Wait(vals ...any) *WaitCell {
	d.lock.Lock()
	defer d.lock.Unlock()
	cell := NewWaitCell(vals...)
	d.Cells = append(d.Cells, cell)
	d.sendLastCell()
	for v := range d.control {
		if v == cell.Id {
			break
		}
	}
	return cell
}

// cell.go

// Not documented. Subject to change.
type WaitCell struct {
	Id     string
	Text   string
	Button string
}

// Not documented. Subject to change.
func (b *WaitCell) Type() string {
	return "WaitCell"
}

// Not documented. Subject to change.
func (b *WaitCell) Append(vals ...any) {
	if len(vals) == 0 {
		return
	}
	b.Text = valsToString(vals[:len(vals)-1])
	b.Button = valsToString(vals[len(vals)-1:])
}

// Not documented. Subject to change.
func (b *WaitCell) Erase() {
	b.Text = ""
	b.Button = ""
}

// Not documented. Subject to change.
func NewWaitCell(vals ...any) *WaitCell {
	cell := &WaitCell{}
	cell.Id = fmt.Sprintf("wait-%p", cell)
	cell.Append(vals...)
	return cell
}
