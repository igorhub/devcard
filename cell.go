package devcard

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Cell is a basic building block of a devcard.
type Cell interface {
	Type() string
	Append(...any)
	Erase()
}

// UnmarshalCell unmarshalls JSON data into a Cell instance.
func UnmarshalCell(cellType string, data []byte) (Cell, error) {
	candidates := []Cell{
		&MarkdownCell{},
		&HTMLCell{},
		&ErrorCell{},
		&MonospaceCell{},
		&ValueCell{},
		&AnnotatedValueCell{},
		&SourceCell{},
		&ImageCell{},
		&JumpCell{},
		&CustomCell{},
		&WaitCell{},
	}

	for _, c := range candidates {
		if cellType == c.Type() {
			err := json.Unmarshal(data, c)
			return c, err
		}
	}
	return nil, fmt.Errorf("unknown type of cell (%s)", cellType)
}

// HTMLCell is a cell with markdown-formatted text.
type HTMLCell struct {
	HTML string `json:"html"`
}

// Returns "HTMLCell". Used for marshaling.
func (c *HTMLCell) Type() string {
	return "HTMLCell"
}

// Append converts vals to strings and appends them to the cell.
func (c *HTMLCell) Append(vals ...any) {
	c.HTML += valsToString(vals)
}

// Erase clears the content of the cell.
func (c *HTMLCell) Erase() {
	c.HTML = ""
}

// NewHTMLCell creates [HTMLCell].
func NewHTMLCell(vals ...any) *HTMLCell {
	c := &HTMLCell{}
	c.Append(vals...)
	return c
}

// MarkdownCell is a cell with markdown-formatted text.
type MarkdownCell struct {
	Text string `json:"text"`
}

// Returns "MarkdownCell". Used for marshaling.
func (c *MarkdownCell) Type() string {
	return "MarkdownCell"
}

// Append converts vals to strings and appends them to the cell.
func (c *MarkdownCell) Append(vals ...any) {
	s := new(strings.Builder)
	for _, val := range vals {
		if str, ok := val.(string); ok {
			s.WriteString(str)
		} else {
			s.WriteString("`" + valToString(val) + "`")
		}
	}
	if c.Text != "" && s.Len() > 0 {
		c.Text += " "
	}
	c.Text += s.String()
}

// Erase clears the content of the cell.
func (c *MarkdownCell) Erase() {
	c.Text = ""
}

// NewMarkdownCell creates [MarkdownCell].
func NewMarkdownCell(vals ...any) *MarkdownCell {
	c := &MarkdownCell{}
	c.Append(vals...)
	return c
}

// ErrorCell is a cell for error reporting.
type ErrorCell struct {
	Title string
	Body  string
}

// Returns "ErrorCell". Used for marshaling.
func (c *ErrorCell) Type() string {
	return "ErrorCell"
}

// Append appends vals to the [ErrorCell].
//
// When the cell is blank, the first of vals becomes the cell's title and the
// rest become its body. If the cell is not blank, all vals become the cell's
// body.
func (c *ErrorCell) Append(vals ...any) {
	switch {
	case c.Title == "" && len(vals) == 1:
		c.Title = valsToString(vals)
	case c.Title == "" && len(vals) > 1:
		c.Title = valsToString(vals[:1])
		c.Body = valsToString(vals[1:])
	case c.Title != "":
		if c.Body != "" {
			c.Body += "\n"
		}
		c.Body += valsToString(vals)
	}
}

// Erase clears the content of the cell.
func (c *ErrorCell) Erase() {
	c.Title = ""
	c.Body = ""
}

// NewErrorCell creates [ErrorCell].
func NewErrorCell(vals ...any) *ErrorCell {
	c := &ErrorCell{}
	c.Append(vals...)
	return c
}

// MonospaceCell is a cell that's supposed to be rendered as monospace, such as block of code.
type MonospaceCell struct {
	Text         string `json:"text"`
	Highlighting string `json:"highlighting"`
}

// Returns "MonospaceCell". Used for marshaling.
func (c *MonospaceCell) Type() string {
	return "MonospaceCell"
}

type monospaceCellOption func(*MonospaceCell)

// WithHighlighting is an option for [Devcard.Mono]. It enables syntax
// highlighting for the code in a [MonospaceCell].
func WithHighlighting(lang string) monospaceCellOption {
	return func(c *MonospaceCell) {
		c.Highlighting = lang
	}
}

// Append converts vals to strings and appends them to the cell.
// [WithHighlighting] option can be used at any position to enable syntax
// highlighting. See [Devcard.Mono] for example.
func (c *MonospaceCell) Append(vals ...any) {
	i := 0
	for _, val := range vals {
		if opt, ok := val.(monospaceCellOption); ok {
			opt(c)
		} else {
			vals[i] = val
			i++
		}
	}
	vals = vals[:i]

	s := valsToString(vals)
	if c.Text != "" {
		c.Text += "\n"
	}
	c.Text += s
}

// Erase clears the content of the cell.
func (c *MonospaceCell) Erase() {
	c.Text = ""
}

// NewMonospaceCell creates [MonospaceCell].
func NewMonospaceCell(vals ...any) *MonospaceCell {
	c := &MonospaceCell{Text: ""}
	c.Append(vals...)
	return c
}

// ValueCell is a cell with pretty-printed Go values.
type ValueCell struct {
	Values []string `json:"values"`
}

// Returns "ValueCell". Used for marshaling.
func (c *ValueCell) Type() string {
	return "ValueCell"
}

// Append appends pretty-printed vals to the cell.
func (c *ValueCell) Append(vals ...any) {
	for _, v := range vals {
		c.Values = append(c.Values, pprint(v))
	}
}

// Erase clears the content of the cell.
func (c *ValueCell) Erase() {
	c.Values = []string{}
}

// NewValueCell creates [ValueCell].
func NewValueCell(vals ...any) *ValueCell {
	c := &ValueCell{Values: []string{}}
	c.Append(vals...)
	return c
}

// AnnotatedValueCell is a cell with pretty-printed Go values that have comments
// attached to them.
type AnnotatedValueCell struct {
	AnnotatedValues []AnnotatedValue `json:"marked_values"`
}

// AnnotatedValueCell contains pretty-printed Go value and its description/annotation.
type AnnotatedValue struct {
	Annotation string `json:"annotation"`
	Value      string `json:"value"`
}

// Returns "AnnotatedValueCell". Used for marshaling.
func (c *AnnotatedValueCell) Type() string {
	return "AnnotatedValueCell"
}

type annotatedVal struct {
	annotation string
	val        any
}

func splitAnnotations(avals []any) []annotatedVal {
	var result []annotatedVal
	for i := 0; i < len(avals); i += 2 {
		var av annotatedVal
		if i+1 < len(avals) {
			av.annotation = valToString(avals[i])
			av.val = avals[i+1]
		} else {
			av.val = avals[i]
		}
		result = append(result, av)
	}
	return result
}

// Append appends one or more AnnotatedValues to the cell. annotationsAndVals
// are converted to annotated values by the rules described in [Devcard.Ann].
func (c *AnnotatedValueCell) Append(annotationsAndVals ...any) {
	for _, av := range splitAnnotations(annotationsAndVals) {
		c.AnnotatedValues = append(c.AnnotatedValues, AnnotatedValue{av.annotation, pprint(av.val)})
	}
}

// Erase clears the content of the cell.
func (c *AnnotatedValueCell) Erase() {
	c.AnnotatedValues = []AnnotatedValue{}
}

// NewAnnotatedValueCell creates [AnnotatedValueCell].
func NewAnnotatedValueCell(annotationsAndVals ...any) *AnnotatedValueCell {
	c := &AnnotatedValueCell{AnnotatedValues: []AnnotatedValue{}}
	c.Append(annotationsAndVals...)
	return c
}

// SourceCell is a cell with source code of a function.
type SourceCell struct {
	Decls []string `json:"decls"`
}

// Returns "SourceCell". Used for marshaling.
func (c *SourceCell) Type() string {
	return "SourceCell"
}

// Append converts vals to strings and appends them to the cell.
func (c *SourceCell) Append(vals ...any) {
	for _, val := range vals {
		c.Decls = append(c.Decls, valToString(val))
	}
}

// Erase clears the content of the cell.
func (c *SourceCell) Erase() {
	c.Decls = c.Decls[0:0:0]
}

// NewSourceCell creates [SourceCell].
func NewSourceCell(decls ...string) *SourceCell {
	c := &SourceCell{}
	for _, decl := range decls {
		c.Append(decl)
	}
	return c
}

// ImageCell is a cell with annotated images.
type ImageCell struct {
	Images []AnnotatedImage `json:"images"`
	Error  *ErrorCell       `json:"error"`

	tempDir string
}

// AnnotatedImage as an image with its description.
type AnnotatedImage struct {
	Annotation string `json:"comment"`
	Path       string `json:"value"`
}

// Returns "ImageCell". Used for marshaling.
func (c *ImageCell) Type() string {
	return "ImageCell"
}

func annotatedImages(tempDir string, vals []any) ([]AnnotatedImage, *ErrorCell) {
	var result []AnnotatedImage
	for _, av := range splitAnnotations(vals) {
		switch x := av.val.(type) {
		case string:
			in, err := os.Open(x)
			if err != nil {
				return nil, NewErrorCell("ImageCell error: unable to read image file", err.Error())
			}
			defer in.Close()
			f, err := os.CreateTemp(tempDir, "temp-image-*"+filepath.Ext(x))
			if err != nil {
				return nil, NewErrorCell("ImageCell error: unable to create a temporary file for an image", err.Error())
			}
			defer f.Close()
			_, err = io.Copy(f, in)
			if err != nil {
				return nil, NewErrorCell("ImageCell error: unable to copy image to the temporary directory", err.Error())
			}
			result = append(result, AnnotatedImage{av.annotation, f.Name()})
		case image.Image:
			f, err := os.CreateTemp(tempDir, "temp-image-*.png")
			if err != nil {
				return nil, NewErrorCell("ImageCell error: unable to create a temporary file for an image", err.Error())
			}
			defer f.Close()
			err = png.Encode(f, x)
			if err != nil {
				return nil, NewErrorCell("ImageCell error: unable to encode an image", err.Error())
			}
			result = append(result, AnnotatedImage{av.annotation, f.Name()})
		case nil:
			panic("image must not be nil")
		default:
			panic("image must be either a path to an image file or an instance of image.Image")
		}
	}
	return result, nil
}

// Append appends one or more AnnotatedImages to the cell. vals are converted to
// annotated images by the rules described in [Devcard.Image].
func (c *ImageCell) Append(vals ...any) {
	// Empty tempDir means we're dealing with a dummy devcard; return immediately.
	if c.tempDir == "" {
		return
	}

	ai, err := annotatedImages(c.tempDir, vals)
	if err != nil {
		c.Error = err
	} else {
		c.Images = append(c.Images, ai...)
	}
}

// Erase clears the content of the cell.
func (c *ImageCell) Erase() {
	c.Images = c.Images[0:0:0]
}

// NewImageCell creates [ImageCell].
func NewImageCell(tempDir string, vals ...any) *ImageCell {
	c := &ImageCell{tempDir: tempDir, Images: []AnnotatedImage{}}
	c.Append(vals...)
	return c
}

type customCell interface {
	Cell
	Cast() Cell
}

// CustomCell provides a base for user-defined cells.
//
// It implements [Cell] interface by providing Type, Append, and Erase methods
// that don't do anything.
type CustomCell struct{}

// Returns "CustomCell".
// Not used anywhere; implemented to satisfy [Cell] interface.
func (c *CustomCell) Type() string {
	return "CustomCell"
}

// Append panics by default. Custom Append might be implemented by user.
func (c *CustomCell) Append(vals ...any) {
	panic("method Append is not implemented for this custom cell")
}

// Erase panics by default. Custom Erase might be implemented by user.
func (c *CustomCell) Erase() {
	panic("method Erase is not implemented for this custom cell")
}

// Custom appends a custom cell to the bottom of the devcard.
//
// The appended HTMLCell is immediately sent to the client.
func (d *Devcard) Custom(cell customCell) {
	d.lock.Lock()
	defer d.lock.Unlock()
	d.Cells = append(d.Cells, cell)
	d.sendLastCell()
}

// Default JumpCell delay, in milliseconds.
var DefaultJumpDelay = 50

// JumpCell is a cell to which we scroll when it's rendered.
type JumpCell struct {
	// Delay in milliseconds.
	Delay int
}

// Returns "JumpCell". Used for marshaling.
func (c *JumpCell) Type() string {
	return "JumpCell"
}

// Noop.
func (c *JumpCell) Append(vals ...any) {
}

// Noop.
func (c *JumpCell) Erase() {
}

// NewJumpCell creates [JumpCell].
func NewJumpCell() *JumpCell {
	cell := &JumpCell{Delay: DefaultJumpDelay}
	return cell
}
