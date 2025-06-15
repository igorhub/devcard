package project

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/igorhub/devcard"
)

func (p *Project) updateFile(path string) ([]byte, error) {
	if filepath.Ext(path) != ".go" {
		return nil, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	file, err := parser.ParseFile(p.fset, path, f, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return data, nil
	}
	p.collectDecls(file)
	p.collectPackage(path, file)
	p.updateDevcardsMeta(path, file)
	return p.rewriteFile(file)
}

func (p *Project) collectDecls(f *ast.File) {
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			p.decls[f.Name.Name+"."+fn.Name.Name] = &printer.CommentedNode{Node: fn, Comments: f.Comments}
		}
	}
}

func (p *Project) collectPackage(path string, f *ast.File) {
	dir, _ := filepath.Split(path)
	relDir, err := filepath.Rel(p.Dir, dir)
	if err != nil {
		panic(fmt.Errorf("collecting package: %w", err))
	}
	p.packages[relDir] = f.Name.Name
}

func (p *Project) source(decl string) (string, error) {
	d, ok := p.decls[decl]
	if !ok {
		return "", errors.New("can't locate the source for " + decl)
	}

	buf := new(bytes.Buffer)
	err := format.Node(buf, p.fset, d)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func importPath(mod, dir, path string) string {
	relPath, err := filepath.Rel(dir, path)
	if err != nil {
		panic(fmt.Errorf("incorrect call to importPath: %w", err))
	}
	parent, _ := filepath.Split(relPath)
	if parent == "" {
		return mod
	}
	return mod + "/" + filepath.Dir(relPath)
}

func (p *Project) updateDevcardsMeta(path string, f *ast.File) {
	p.cardsMeta = slices.DeleteFunc(p.cardsMeta, func(meta devcard.DevcardMeta) bool {
		return filepath.Join(p.Dir, meta.Path) == path
	})

	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && isDevcardProducer(p.fset, fn) {
			devcardPath, err := filepath.Rel(p.Dir, path)
			if err != nil {
				// We can't reach here, but let's panic just in case.
				panic(fmt.Errorf("updateDevcardsMeta: %w", err))
			}
			meta := devcard.DevcardMeta{
				ImportPath: importPath(p.Module, p.Dir, path),
				Package:    f.Name.Name,
				Path:       devcardPath,
				Line:       p.fset.Position(fn.Pos()).Line,
				Name:       fn.Name.Name,
				Title:      devcardTitle(p.fset, fn),
			}
			p.cardsMeta = append(p.cardsMeta, meta)
		}
	}

	slices.SortStableFunc(p.cardsMeta, func(a, b devcard.DevcardMeta) int { return cmp.Compare(a.Path, b.Path) })
}

func (p *Project) rewriteFile(f *ast.File) ([]byte, error) {
	for _, decl := range f.Decls {
		if f, ok := decl.(*ast.FuncDecl); ok && f.Name.Name == "main" {
			f.Name.Name = "_main_orig"
		}
	}

	buf := new(bytes.Buffer)
	err := format.Node(buf, p.fset, f)
	if err != nil {
		err = fmt.Errorf("rewriting: %w", err)
	}
	return buf.Bytes(), err
}

func isDevcardProducer(fset *token.FileSet, fn *ast.FuncDecl) bool {
	if !strings.HasPrefix(fn.Name.Name, "Devcard") {
		return false
	}

	if fn.Type.TypeParams != nil {
		return false
	}

	if fn.Type.Results != nil {
		return false
	}

	if len(fn.Type.Params.List) != 1 {
		return false
	}

	s := new(strings.Builder)
	format.Node(s, fset, fn.Type.Params.List[0].Type)
	return s.String() == "*devcard.Devcard"
}

func devcardTitle(fset *token.FileSet, fn *ast.FuncDecl) string {
	for _, stmt := range fn.Body.List {
		expr, ok := stmt.(*ast.ExprStmt)
		if !ok {
			continue
		}

		x, ok := expr.X.(*ast.CallExpr)
		if !ok {
			continue
		}

		fun, ok := x.Fun.(*ast.SelectorExpr)
		if !ok {
			continue
		}

		if fun.Sel.Name != "SetTitle" || len(x.Args) != 1 {
			continue
		}

		buf := new(bytes.Buffer)
		format.Node(buf, fset, x.Args[0])
		s := buf.String()
		if _, ok := x.Args[0].(*ast.BasicLit); ok && len(s) > 1 {
			s = s[1 : len(s)-1]
		}
		s = strings.ReplaceAll(s, "\\\"", "\"")
		return s
	}

	return ""
}
