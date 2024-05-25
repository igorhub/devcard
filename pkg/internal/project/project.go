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
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/igorhub/devcard"
	"github.com/igorhub/devcard/pkg/internal/file"
	"golang.org/x/mod/modfile"
)

const bundlingPeriod = 30 * time.Millisecond

type Project struct {
	Name   string
	Dir    string
	Module string

	Err    error
	Update chan struct{}

	cardsInfo []devcard.DevcardInfo
	cache     map[string][]byte
	events    chan projectMessage
	clones    map[string]bool

	fset  *token.FileSet
	decls map[string]*printer.CommentedNode
}

func NewProject(name, dir string) *Project {
	p := &Project{
		Name:   name,
		Dir:    dir,
		Update: make(chan struct{}, 256),
		cache:  make(map[string][]byte),
		events: make(chan projectMessage, 256),
		clones: make(map[string]bool),

		fset:  token.NewFileSet(),
		decls: make(map[string]*printer.CommentedNode),
	}
	mod, err := moduleName(dir)
	if err != nil {
		p.Err = err
		return p
	}
	p.Module = mod
	p.startWatching()
	return p
}

func (p *Project) CreateRepo(devcardName string) (*Repo, error) {
	resultC, errC := make(chan *Repo), make(chan error)
	p.events <- msgCreateRepo{devcardName: devcardName, result: resultC, err: errC}
	return <-resultC, <-errC
}

func (p *Project) DevcardInfo(devcardName string) devcard.DevcardInfo {
	if p == nil {
		return devcard.DevcardInfo{}
	}
	resultC := make(chan []devcard.DevcardInfo)
	p.events <- msgGetDevcards{resultC}
	info := <-resultC
	return lookupDevcardInfo(info, devcardName)
}

func (p *Project) DevcardsInfo() []devcard.DevcardInfo {
	if p == nil {
		return nil
	}
	resultC := make(chan []devcard.DevcardInfo)
	p.events <- msgGetDevcards{resultC}
	return <-resultC
}

func (p *Project) Source(decl string) (string, error) {
	if p == nil {
		return "", nil
	}
	resultC := make(chan msgGetSource)
	p.events <- msgGetSource{decl: decl, result: resultC}
	result := <-resultC
	return result.source, result.err
}

func (p *Project) RemoveRepo(repo *Repo) {
	if repo != nil {
		p.events <- msgRemoveRepo{repo.Dir}
	}
}

func (p *Project) Shutdown() {
	done := make(chan struct{})
	p.events <- msgShutdown{done}
	<-done
}

type projectMessage interface{ projectMessage() }

type msgCreateRepo struct {
	devcardName string
	result      chan<- *Repo
	err         chan<- error
}

type msgRemoveRepo struct {
	repoDir string
}

type msgUpdateFile struct {
	path string
}

type msgRemoveFile struct {
	path string
}

type msgGetDevcards struct {
	result chan<- []devcard.DevcardInfo
}

type msgGetSource struct {
	decl   string
	source string
	err    error
	result chan<- msgGetSource
}

type msgRestart struct{}

type msgFail struct {
	err error
}

type msgShutdown struct {
	done chan<- struct{}
}

func (msgCreateRepo) projectMessage()  {}
func (msgRemoveRepo) projectMessage()  {}
func (msgUpdateFile) projectMessage()  {}
func (msgRemoveFile) projectMessage()  {}
func (msgGetDevcards) projectMessage() {}
func (msgGetSource) projectMessage()   {}
func (msgRestart) projectMessage()     {}
func (msgFail) projectMessage()        {}
func (msgShutdown) projectMessage()    {}

func (p *Project) startWatching() {
	watcher, err := startWatcher(p.Dir, p.events)
	if err != nil {
		p.Err = err
		return
	}

	p.decls = make(map[string]*printer.CommentedNode)
	err = filepath.WalkDir(p.Dir, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir():
			return nil
		default:
			return p.updateFile(path)
		}
	})
	if err != nil {
		p.Err = err
		return
	}

	for r := range p.clones {
		err := p.syncDir(r)
		if err != nil {
			p.events <- msgFail{err}
			break
		}
	}
	p.Update <- struct{}{}

	go func() {
		update := make(chan struct{}, cap(p.Update))
		bundle(bundlingPeriod, update, p.Update)
		defer close(update)

		for e := range p.events {
			log.Printf("project %s event: %#v", filepath.Base(p.Dir), e)

			switch e := e.(type) {
			case msgCreateRepo:
				repo, err := p.createRepo(e.devcardName)
				e.result <- repo
				close(e.result)
				e.err <- err
				close(e.err)

			case msgRemoveRepo:
				os.RemoveAll(e.repoDir)
				delete(p.clones, e.repoDir)

			case msgUpdateFile:
				err := p.updateFile(e.path)
				if err != nil {
					p.events <- msgFail{err}
				}
				for repo := range p.clones {
					p.syncFile(e.path, repo)
				}
				update <- struct{}{}

			case msgRemoveFile:
				p.removeFile(e.path)
				for repo := range p.clones {
					path := replaceRootDir(p.Dir, repo, e.path)
					os.Remove(path)
				}
				update <- struct{}{}

			case msgGetDevcards:
				e.result <- slices.Clone(p.cardsInfo)
				close(e.result)

			case msgGetSource:
				e.source, e.err = p.source(e.decl)
				e.result <- e
				close(e.result)

			case msgRestart:
				watcher.Close()
				p.startWatching()
				return

			case msgFail:
				p.Err = e.err
				watcher.Close()
				update <- struct{}{}
				return

			case msgShutdown:
				for dir := range p.clones {
					os.RemoveAll(dir)
				}
				watcher.Close()
				close(e.done)
				return
			}
		}
	}()
}

func (p *Project) createRepo(devcardName string) (*Repo, error) {
	info := lookupDevcardInfo(p.cardsInfo, devcardName)
	if info == (devcard.DevcardInfo{}) {
		return nil, fmt.Errorf("create repo: devcard %s not found in %s", devcardName, p.Name)
	}

	repo, err := newRepo(p, info)
	if err != nil {
		return nil, err
	}

	p.clones[repo.Dir] = true
	err = p.syncDir(repo.Dir)
	if err != nil {
		p.events <- msgFail{err}
		return nil, err
	}

	return repo, nil
}

func lookupDevcardInfo(cards []devcard.DevcardInfo, devcardName string) devcard.DevcardInfo {
	i := slices.IndexFunc(cards, func(info devcard.DevcardInfo) bool {
		return info.Name == devcardName
	})
	if i == -1 {
		return devcard.DevcardInfo{}
	}
	return cards[i]
}

func (p *Project) syncDir(repoDir string) error {
	err := filepath.WalkDir(p.Dir, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.Name() == ".git":
			return fs.SkipDir
		case d.IsDir():
			_ = os.Mkdir(replaceRootDir(p.Dir, repoDir, path), 0700)
			return nil
		default:
			return p.syncFile(path, repoDir)
		}
	})
	if err != nil {
		return fmt.Errorf("sync repo %s for %s: %w", repoDir, p.Name, err)
	}
	return nil
}

func (p *Project) syncFile(path, repoDir string) error {
	dst := replaceRootDir(p.Dir, repoDir, path)
	if data := p.cache[path]; data != nil {
		return os.WriteFile(dst, data, 0600)
	}
	return linkOrCopy(path, dst)
}

func replaceRootDir(dirFrom, dirTo, path string) string {
	if dirTo == "" {
		panic("dirTo must not be empty")
	}
	if dirTo == dirFrom {
		panic("dirTo must not be the same as dirFrom")
	}
	rel, err := filepath.Rel(dirFrom, path)
	if err != nil {
		panic(fmt.Errorf("path %q must be located in %q", path, dirFrom))
	}
	return filepath.Join(dirTo, rel)
}

func linkOrCopy(src, dst string) error {
	os.Remove(dst)
	err := os.Link(src, dst)
	if err != nil {
		err = file.Copy(src, dst)
	}
	return err
}

func (p *Project) updateFile(path string) error {
	if filepath.Ext(path) != ".go" {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	file, err := parser.ParseFile(p.fset, path, f, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		p.cache[path] = data
		return nil
	}
	p.collectDecls(file)
	p.updateDevcardsInfo(path, file)
	data, err := p.rewriteFile(file)
	if err != nil {
		return err
	}
	p.cache[path] = data
	return nil
}

func (p *Project) collectDecls(f *ast.File) {
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			p.decls[f.Name.Name+"."+fn.Name.Name] = &printer.CommentedNode{Node: fn, Comments: f.Comments}
		}
	}
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

func (p *Project) updateDevcardsInfo(path string, f *ast.File) {
	p.cardsInfo = slices.DeleteFunc(p.cardsInfo, func(info devcard.DevcardInfo) bool {
		return filepath.Join(p.Dir, info.Path) == path
	})

	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && isDevcardProducer(p.fset, fn) {
			devcardPath, err := filepath.Rel(p.Dir, path)
			if err != nil {
				// We can't reach here, but let's panic just in case.
				panic(fmt.Errorf("updateDevcardsInfo: %w", err))
			}
			info := devcard.DevcardInfo{
				ImportPath: importPath(p.Module, p.Dir, path),
				Package:    f.Name.Name,
				Path:       devcardPath,
				Line:       p.fset.Position(fn.Pos()).Line,
				Name:       fn.Name.Name,
				Title:      devcardTitle(p.fset, fn),
			}
			p.cardsInfo = append(p.cardsInfo, info)
		}
	}

	slices.SortStableFunc(p.cardsInfo, func(a, b devcard.DevcardInfo) int { return cmp.Compare(a.Path, b.Path) })
}

func (p *Project) rewriteFile(f *ast.File) ([]byte, error) {
	for _, decl := range f.Decls {
		if f, ok := decl.(*ast.FuncDecl); ok && f.Name.Name == "main" {
			f.Name.Name = "_main_orig"
		}
	}

	buf := new(bytes.Buffer)
	err := format.Node(buf, p.fset, f)
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
		return s
	}

	return ""
}

func (p *Project) removeFile(path string) {
	p.cardsInfo = slices.DeleteFunc(p.cardsInfo, func(info devcard.DevcardInfo) bool {
		return filepath.Join(p.Dir, info.Path) == path
	})
	delete(p.cache, path)
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

func startWatcher(dir string, events chan projectMessage) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("start watcher in %q: %w", dir, err)
	}

	watchDirs, err := subdirs(dir)
	if err != nil {
		return nil, fmt.Errorf("start watcher: %w", err)
	}
	slices.Sort(watchDirs)
	for _, dir := range watchDirs {
		err = watcher.Add(dir)
		if err != nil {
			watcher.Close()
			return nil, fmt.Errorf("start watcher in %q: %w", dir, err)
		}
	}

	isDir := func(path string) bool {
		if _, ok := slices.BinarySearch(watchDirs, path); ok {
			return true
		}
		info, err := os.Stat(path)
		if err != nil {
			return false
		}
		return info.IsDir()
	}

	go func() {
		for {
			select {
			case e, ok := <-watcher.Events:
				if !ok {
					return
				}

				switch e.Op {
				case fsnotify.Create, fsnotify.Write:
					if isDir(e.Name) {
						events <- msgRestart{}
					} else {
						events <- msgUpdateFile{path: e.Name}
					}
				case fsnotify.Remove, fsnotify.Rename:
					if isDir(e.Name) {
						events <- msgRestart{}
					} else {
						events <- msgRemoveFile{path: e.Name}
					}
				default:
					// Ignore fsnotify.Chmod, as recommended by fsnotify docs.
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				events <- msgFail{err: fmt.Errorf("%s watcher error: %w", dir, err)}
			}
		}
	}()

	return watcher, nil
}

func subdirs(projectDir string) ([]string, error) {
	var result []string
	if !filepath.IsAbs(projectDir) {
		panic(fmt.Errorf("projectDir %q must be an absolute path", projectDir))
	}
	err := filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case !d.IsDir():
			return nil
		case d.Name() == ".git":
			return filepath.SkipDir
		}
		result = append(result, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("building directory structure of %s: %w", projectDir, err)
	}
	return result, nil
}

func moduleName(projectDir string) (string, error) {
	path := filepath.Join(projectDir, "go.mod")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	mod, err := modfile.Parse(path, data, nil)
	if err != nil {
		return "", err
	}
	return mod.Module.Syntax.Token[1], nil
}
