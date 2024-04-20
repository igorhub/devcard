// Package devcard describes the [Devcard] and its primary building block, [Cell].
//
// For proper introduction, see README.
package devcard

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"sync"
)

// Devcard struct represents the devcard shown to the user.
//
// It's responsible for maintaining its list of renderable cells and for
// interaction with the server.
//
// It's safe for concurrent use.
type Devcard struct {
	Title   string `json:"title"`
	TempDir string `json:"temp_dir"`
	Cells   []Cell `json:"cells"`

	lock    sync.RWMutex
	control chan string
	updates chan string
}

func newDevcard(title, tempDir string) *Devcard {
	return &Devcard{
		Title:   title,
		TempDir: tempDir,
		Cells:   []Cell{},

		control: make(chan string),
		updates: make(chan string, 4096),
	}
}

// Debug facilitates debugging. To debug a devcard, either put a call to Debug
// in the main(), or wrap it in a test function, and then use "Debug Test"
// feature of your IDE.
//
// Example:
//
//	func TestDevcardFoobar(t *testing.T) {
//		devcard.Debug(DevcardFoobar)
//	}
func Debug(producer DevcardProducer) {
	current = &Devcard{
		Title:   "Dummy devcard",
		TempDir: "",
		Cells:   []Cell{},
	}
	producer(current)
}

// DevcardInfo describes devcard's metadata.
type DevcardInfo struct {
	// ImportPath is the import path of the devcard's package.
	ImportPath string

	// Package is the name of the package that the devcard belongs to.
	Package string

	// Path is the relative path (from the project dir) to the source file where
	// the devcard is located.
	Path string

	// Line is a line number in the source file where the devcard is located.
	Line int

	// Name is the name of the devcard-producing function.
	Name string

	// Title is the title of the devcard.
	Title string
}

// Caption returns the devcard's title, or, in case it's empty, the name of
// devcard-producing function.
func (di DevcardInfo) Caption() string {
	if di.Title != "" {
		return di.Title
	}
	return di.Name
}

var current *Devcard

// Current returns a global pointer to the devcard that's currently being
// produced. It's exposed to allow the user to access the current devcard from
// any arbitrary place.
func Current() *Devcard {
	return current
}

// Message types are used for communication with devcards server via TCP connection.
const (
	MessageTypeCell  = "cell"
	MessageTypeInfo  = "info"
	MessageTypeError = "internal error"
)

func (d *Devcard) send(msg map[string]any) {
	if d.updates == nil {
		return
	}
	data, err := json.Marshal(msg)
	if err != nil {
		data, _ = json.Marshal(map[string]string{
			"msg_type": MessageTypeError,
			"error":    err.Error(),
		})
	}
	d.updates <- string(data)
}

func (d *Devcard) sendCell(index int) {
	cell := d.Cells[index]
	if customCell, ok := cell.(customCell); ok {
		cell = customCell.Cast()
	}
	d.send(map[string]any{
		"msg_type":  MessageTypeCell,
		"cell_type": cell.Type(),
		"id":        "b" + strconv.Itoa(index),
		"cell":      cell,
	})
}

func (d *Devcard) sendLastCell() {
	d.sendCell(len(d.Cells) - 1)
}

func (d *Devcard) sendInfo() {
	d.send(map[string]any{
		"msg_type": MessageTypeInfo,
		"title":    d.Title,
	})
}

// SetTitle sets the devcard's title and updates it on the client.
func (d *Devcard) SetTitle(title string) {
	d.lock.Lock()
	defer d.lock.Unlock()
	d.Title = title
	d.sendInfo()
}

// Md appends a [MarkdownCell] to the bottom of the devcard. vals are converted into
// strings and concatenated.
//
// The appended MarkdownCell is immediately sent to the client.
func (d *Devcard) Md(vals ...any) *MarkdownCell {
	d.lock.Lock()
	defer d.lock.Unlock()
	cell := NewMarkdownCell(vals...)
	d.Cells = append(d.Cells, cell)
	d.sendLastCell()
	return cell
}

// Html appends an [HTMLCell] to the bottom of the devcard. vals are converted
// into strings and concatenated.
//
// The appended HTMLCell is immediately sent to the client.
func (d *Devcard) Html(vals ...any) *HTMLCell {
	d.lock.Lock()
	defer d.lock.Unlock()
	cell := NewHTMLCell(vals...)
	d.Cells = append(d.Cells, cell)
	d.sendLastCell()
	return cell
}

// MdFmt is a convenience wrapper for [Devcard.Md].
//
// It's implemented as `return d.Md(fmt.Sprintf(format, a...))`.
func (d *Devcard) MdFmt(format string, a ...any) *MarkdownCell {
	return d.Md(fmt.Sprintf(format, a...))
}

// Error appends an [ErrorCell] to the bottom of the devcard.
//
// The first of vals becomes the cell's title; the rest are converted into
// strings, concatenated, and become the cell's body.
//
// The appended ErrorCell is immediately sent to the client.
func (d *Devcard) Error(vals ...any) *ErrorCell {
	d.lock.Lock()
	defer d.lock.Unlock()
	cell := NewErrorCell(vals...)
	d.Cells = append(d.Cells, cell)
	d.sendLastCell()
	return cell
}

// Mono appends a [MonospaceCell] to the bottom of the devcard. vals are
// converted into strings and concatenated.
//
// [WithHighlighting] option can be used at any position to enable syntax highlighting. For example:
//
//	c.Mono(devcard.WithHighlighting("clojure"), "(def ^:private *registry (atom {}))")
//
// The appended MonospaceCell is immediately sent to the client.
func (d *Devcard) Mono(vals ...any) *MonospaceCell {
	d.lock.Lock()
	defer d.lock.Unlock()
	cell := NewMonospaceCell(vals...)
	d.Cells = append(d.Cells, cell)
	d.sendLastCell()
	return cell
}

// MonoFmt is a convenience wrapper for Mono.
//
// It's implemented as `return d.Mono(fmt.Sprintf(format, a...))`.
func (d *Devcard) MonoFmt(format string, a ...any) *MonospaceCell {
	return d.Mono(fmt.Sprintf(format, a...))
}

// Val appends a [ValueCell] to the bottom of the devcard. vals are
// pretty-printed and joined together.
//
// The appended ValueCell is immediately sent to the client.
func (d *Devcard) Val(vals ...any) *ValueCell {
	d.lock.Lock()
	defer d.lock.Unlock()
	cell := NewValueCell(vals...)
	d.Cells = append(d.Cells, cell)
	d.sendLastCell()
	return cell
}

// Ann appends an [AnnotatedValueCell] to the bottom of the devcard.
// annotationsAndVals are split into pairs: the first value of each pair becomes
// an annotation, the second value becomes a pretty-printed value.
//
// Example:
//
//	c.Ann("Loaded config:", cfg, "Default config:", defaultConfig())
//
// The appended AnnotatedValueCell is immediately sent to the client.
func (d *Devcard) Ann(annotationsAndVals ...any) *AnnotatedValueCell {
	d.lock.Lock()
	defer d.lock.Unlock()
	cell := NewAnnotatedValueCell(annotationsAndVals...)
	d.Cells = append(d.Cells, cell)
	d.sendLastCell()
	return cell
}

// Source appends a [SourceCell] to the bottom of the devcard.
//
// The cell contains the source of the declarations decls. As of now, only
// function declarations are supported. Declarations must be prefixed with the
// name of their package. For example:
//
//	c.Source("examples.DevcardTextCells")
//
// The appended SourceCell is immediately sent to the client.
func (d *Devcard) Source(decls ...string) *SourceCell {
	d.lock.Lock()
	defer d.lock.Unlock()
	cell := NewSourceCell(decls...)
	d.Cells = append(d.Cells, cell)
	d.sendLastCell()
	return cell
}

// Image appends an [ImageCell] to the bottom of the devcard. annotationsAndVals
// are split into pairs: the first value of each pair becomes an annotation, the
// second value becomes an image.
//
// An image can be either an absolute path to the image file, or an instance of
// [image.Image].
//
// When called with a single argument, the argument is treated as image, not
// annotation. For example:
//
//	c.Image("/home/ivk/Pictures/wallhaven-n6mrgl.jpg")
//
//	// With annotation
//	c.Image("Two cats sitting on a tree", "/home/ivk/Pictures/wallhaven-n6mrgl.jpg")
//
// The appended ImageCell is immediately sent to the client.
func (d *Devcard) Image(annotationsAndImages ...any) *ImageCell {
	d.lock.Lock()
	defer d.lock.Unlock()
	cell := NewImageCell(d.TempDir)
	cell.Append(annotationsAndImages...)
	d.Cells = append(d.Cells, cell)
	d.sendLastCell()
	return cell
}

// Not documented. Subject to change.
func (d *Devcard) Jump() *JumpCell {
	d.lock.Lock()
	defer d.lock.Unlock()
	cell := NewJumpCell()
	d.Cells = append(d.Cells, cell)
	d.sendLastCell()
	return cell
}

// Append appends values to the bottom cell of the devcard. The exact behavior
// is dictated by the concrete type of the bottom cell.
//
//   - For [MarkdownCell], same rules as in [Devcard.Md] apply.
//   - For [ErrorCell], same rules as in [Devcard.Error] apply.
//   - For [MonospaceCell], same rules as in [Devcard.Mono] apply.
//   - For [ValueCell], same rules as in [Devcard.Val] apply.
//   - Fro [AnnotatedValueCell], same rules as in [Devcard.Ann] apply.
//   - For [ImageCell], same rules as in [Devcard.Image] apply.
//   - For other types of cells, Append is a noop.
//
// The bottom cell is immediately sent to the client.
func (d *Devcard) Append(vals ...any) {
	d.lock.Lock()
	defer d.lock.Unlock()
	i := len(d.Cells) - 1
	if i < 0 {
		// If there are no cells yet, create a new one.
		d.Cells = append(d.Cells, NewMonospaceCell())
		i = 0
	}
	d.Cells[i].Append(vals...)
	d.sendLastCell()
}

type cellError struct {
	cell Cell
}

func (e *cellError) Error() string {
	return "this cell is not included in the devcard"
}

// Erase clears the content of the cell.
//
// The cell is not removed from the devcard, and can be reused later on.
//
// The resulting blank cell is immediately sent to the client.
func (d *Devcard) Erase(cell Cell) {
	d.lock.Lock()
	defer d.lock.Unlock()
	i := slices.Index(d.Cells, cell)
	if i == -1 {
		panic(&cellError{cell})
	}
	d.Cells[i].Erase()
	d.sendCell(i)
}

// EraseLast clears the content of the bottom cell of the devcard.
//
// The cell is not removed from the devcard, and can be reused later on.
//
// The blank bottom cell is immediately sent to the client.
func (d *Devcard) EraseLast() {
	d.lock.Lock()
	defer d.lock.Unlock()
	i := len(d.Cells) - 1
	if i < 0 {
		return
	}
	d.Cells[i].Erase()
	d.sendLastCell()
}

// Replace replaces oldCell with newCell.
//
// The new cell is immediately sent to the client.
func (d *Devcard) Replace(oldCell, newCell Cell) {
	d.lock.Lock()
	defer d.lock.Unlock()
	i := slices.Index(d.Cells, oldCell)
	if i == -1 {
		panic(&cellError{oldCell})
	}
	d.Cells[i] = newCell
	d.sendCell(i)
}

// ReplaceLast replaces the bottom cell of the devcard with newCell.
//
// The new cell is immediately sent to the client.
func (d *Devcard) ReplaceLast(newCell Cell) {
	d.lock.Lock()
	defer d.lock.Unlock()
	i := len(d.Cells) - 1
	if i < 0 {
		return
	}
	d.Cells[i] = newCell
	d.sendLastCell()
}

// Update sends the cell to the client.
//
// The cell must be contained by the devcard.
func (d *Devcard) Update(cell Cell) {
	d.lock.Lock()
	defer d.lock.Unlock()
	i := slices.Index(d.Cells, cell)
	if i == -1 {
		panic(&cellError{cell})
	}
	d.sendCell(i)
}

// MarshalJSON marshals the devcard into JSON data.
func (d *Devcard) MarshalJSON() ([]byte, error) {
	d.lock.RLock()
	defer d.lock.RUnlock()
	jsoncells := make([]struct {
		Type string `json:"type"`
		Cell Cell   `json:"cell"`
	}, len(d.Cells))

	for i, c := range d.Cells {
		jsoncells[i].Type = c.Type()
		jsoncells[i].Cell = c
	}

	return json.Marshal(map[string]any{
		"title": d.Title,
		"cells": jsoncells,
	})
}

// UnmarshalJSON unmarshals JSON data into the devcard.
func (d *Devcard) UnmarshalJSON(data []byte) error {
	d.lock.Lock()
	defer d.lock.Unlock()

	type jsoncell struct {
		Type string           `json:"type"`
		Cell *json.RawMessage `json:"cell"`
	}

	jsondevcard := struct {
		Title string     `json:"title"`
		Cells []jsoncell `json:"cells"`
	}{}

	err := json.Unmarshal(data, &jsondevcard)
	if err != nil {
		return fmt.Errorf("unmarshal devcard: %w", err)
	}

	d.Title = jsondevcard.Title
	for _, c := range jsondevcard.Cells {
		if c.Cell == nil {
			return fmt.Errorf("unmarshal devcard cell: nil cell")
		}
		cell, err := UnmarshalCell(c.Type, *c.Cell)
		if err != nil {
			return fmt.Errorf("unmarshal devcard cell: %w", err)
		}
		d.Cells = append(d.Cells, cell)
	}

	return nil
}
