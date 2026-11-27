// Package resolve turns code refs into absolute paths and line ranges. Go
// symbols are resolved with go/packages; other files use `lines`.
package resolve

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/tools/go/packages"

	"github.com/rrgmc/ai-contextview/internal/protocol"
)

// Resolver resolves code refs.
type Resolver interface {
	Resolve(ctx context.Context, cwd string, ref protocol.CodeRef) (protocol.ResolvedRef, error)
}

// New returns the default resolver with a package cache.
func New() *GoResolver { return &GoResolver{cache: map[string]*cacheEntry{}} }

// GoResolver resolves `lines` refs for any file and `symbol` refs for Go.
type GoResolver struct {
	mu    sync.Mutex
	cache map[string]*cacheEntry
}

type cacheEntry struct {
	fset   *token.FileSet
	pkgs   []*packages.Package
	mtimes map[string]time.Time
}

// Resolve implements Resolver.
func (g *GoResolver) Resolve(ctx context.Context, cwd string, ref protocol.CodeRef) (protocol.ResolvedRef, error) {
	if ref.Path == "" {
		return protocol.ResolvedRef{}, errors.New("code ref without path")
	}
	if (ref.Symbol == "") == (ref.Lines == "") {
		return protocol.ResolvedRef{}, fmt.Errorf("%s: give exactly one of symbol or lines", ref.Path)
	}
	abs, err := AbsPath(cwd, ref.Path)
	if err != nil {
		return protocol.ResolvedRef{}, err
	}
	if ref.Lines != "" {
		return resolveLines(abs, ref.Lines)
	}
	if filepath.Ext(abs) != ".go" {
		return protocol.ResolvedRef{}, fmt.Errorf("%s: symbols are only supported for Go; use lines", ref.Path)
	}
	return g.resolveSymbol(ctx, abs, ref.Symbol)
}

// AbsPath resolves a path against cwd. Relative paths must stay inside the
// git root (or cwd when there is none); absolute paths must exist.
func AbsPath(cwd, p string) (string, error) {
	if filepath.IsAbs(p) {
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("%s does not exist", p)
		}
		return filepath.Clean(p), nil
	}
	abs := filepath.Join(cwd, p)
	root := GitRoot(cwd)
	if root == "" {
		root = cwd
	}
	if rel, err := filepath.Rel(root, abs); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s is outside the project (%s); use an absolute path", p, root)
	}
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("%s does not exist", p)
	}
	return abs, nil
}

// GitRoot walks up from dir looking for .git; it returns "" when not found.
func GitRoot(dir string) string {
	for d := dir; ; {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

func countLines(abs string) (int, error) {
	data, err := os.ReadFile(abs)
	if err != nil {
		return 0, err
	}
	n := bytes.Count(data, []byte("\n"))
	if len(data) > 0 && data[len(data)-1] != '\n' {
		n++
	}
	if n == 0 {
		n = 1
	}
	return n, nil
}

// ParseLines parses "N" or "N-M".
func ParseLines(s string) (int, int, error) {
	a, b, isRange := strings.Cut(strings.TrimSpace(s), "-")
	start, err := strconv.Atoi(strings.TrimSpace(a))
	if err != nil || start < 1 {
		return 0, 0, fmt.Errorf("bad lines %q: want N or N-M", s)
	}
	end := start
	if isRange {
		end, err = strconv.Atoi(strings.TrimSpace(b))
		if err != nil || end < start {
			return 0, 0, fmt.Errorf("bad lines %q: want N or N-M with N <= M", s)
		}
	}
	return start, end, nil
}

func resolveLines(abs, lines string) (protocol.ResolvedRef, error) {
	start, end, err := ParseLines(lines)
	if err != nil {
		return protocol.ResolvedRef{}, err
	}
	n, err := countLines(abs)
	if err != nil {
		return protocol.ResolvedRef{}, err
	}
	if start > n {
		start = n
	}
	if end > n {
		end = n
	}
	return protocol.ResolvedRef{Path: abs, StartLine: start, EndLine: end, Source: "lines"}, nil
}

// ------------------------------------------------------------------ symbols

type symbolName struct {
	recv string // receiver type for methods, "" otherwise
	name string
}

// parseSymbol parses pkg.Func, Func, pkg.(*T).M, (*T).M, pkg.T.M, T.M. The
// package qualifier is dropped when it matches pkgName.
func parseSymbol(sym, pkgName string) ([]symbolName, error) {
	s := strings.TrimSpace(sym)
	if s == "" {
		return nil, errors.New("empty symbol")
	}
	// Drop a full import path prefix ("github.com/x/y/sync.Func").
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	if strings.HasPrefix(s, pkgName+".") {
		s = s[len(pkgName)+1:]
	}
	if strings.HasPrefix(s, "(") {
		end := strings.Index(s, ")")
		if end < 0 || end+2 > len(s) || s[end+1] != '.' {
			return nil, fmt.Errorf("bad symbol %q", sym)
		}
		recv := strings.TrimPrefix(strings.TrimSpace(s[1:end]), "*")
		return []symbolName{{recv: stripTypeParams(recv), name: s[end+2:]}}, nil
	}
	parts := strings.Split(s, ".")
	switch len(parts) {
	case 1:
		return []symbolName{{name: parts[0]}}, nil
	case 2:
		// Either Type.Method, or another package's name followed by a name.
		return []symbolName{{recv: stripTypeParams(parts[0]), name: parts[1]}, {name: parts[1]}}, nil
	case 3:
		return []symbolName{{recv: stripTypeParams(parts[1]), name: parts[2]}}, nil
	}
	return nil, fmt.Errorf("bad symbol %q", sym)
}

func stripTypeParams(s string) string {
	if i := strings.Index(s, "["); i >= 0 {
		return s[:i]
	}
	return s
}

func (g *GoResolver) load(ctx context.Context, abs string) (*cacheEntry, error) {
	dir := filepath.Dir(abs)
	g.mu.Lock()
	defer g.mu.Unlock()
	if e, ok := g.cache[dir]; ok && !stale(dir, e.mtimes) {
		return e, nil
	}
	modRoot := dir
	for d := dir; ; {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			modRoot = d
			break
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	fset := token.NewFileSet()
	cfg := &packages.Config{
		Context: ctx,
		Mode:    packages.NeedName | packages.NeedFiles | packages.NeedSyntax,
		Dir:     modRoot,
		Fset:    fset,
		Tests:   strings.HasSuffix(abs, "_test.go"),
	}
	pkgs, err := packages.Load(cfg, "file="+abs)
	if err != nil {
		return nil, fmt.Errorf("loading package for %s: %w", abs, err)
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no Go package contains %s", abs)
	}
	e := &cacheEntry{fset: fset, pkgs: pkgs, mtimes: dirMtimes(dir)}
	g.cache[dir] = e
	return e, nil
}

func dirMtimes(dir string) map[string]time.Time {
	out := map[string]time.Time{}
	entries, _ := os.ReadDir(dir)
	for _, en := range entries {
		if strings.HasSuffix(en.Name(), ".go") {
			if info, err := en.Info(); err == nil {
				out[en.Name()] = info.ModTime()
			}
		}
	}
	return out
}

func stale(dir string, old map[string]time.Time) bool {
	now := dirMtimes(dir)
	if len(now) != len(old) {
		return true
	}
	for k, v := range now {
		if !old[k].Equal(v) {
			return true
		}
	}
	return false
}

type decl struct {
	recv, name, kind string
	start, end       token.Pos
}

func collectDecls(files []*ast.File) []decl {
	var out []decl
	for _, f := range files {
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				start := d.Pos()
				if d.Doc != nil {
					start = d.Doc.Pos()
				}
				if d.Recv != nil && len(d.Recv.List) > 0 {
					out = append(out, decl{recv: recvName(d.Recv.List[0].Type), name: d.Name.Name, kind: "method", start: start, end: d.End()})
				} else {
					out = append(out, decl{name: d.Name.Name, kind: "func", start: start, end: d.End()})
				}
			case *ast.GenDecl:
				single := len(d.Specs) == 1 && !d.Lparen.IsValid()
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						start, end := s.Pos(), s.End()
						if s.Doc != nil {
							start = s.Doc.Pos()
						}
						if single {
							start, end = d.Pos(), d.End()
							if d.Doc != nil {
								start = d.Doc.Pos()
							}
						}
						out = append(out, decl{name: s.Name.Name, kind: "type", start: start, end: end})
					case *ast.ValueSpec:
						kind := "var"
						if d.Tok == token.CONST {
							kind = "const"
						}
						start, end := s.Pos(), s.End()
						if s.Doc != nil {
							start = s.Doc.Pos()
						}
						if single {
							start, end = d.Pos(), d.End()
							if d.Doc != nil {
								start = d.Doc.Pos()
							}
						}
						for _, n := range s.Names {
							out = append(out, decl{name: n.Name, kind: kind, start: start, end: end})
						}
					}
				}
			}
		}
	}
	return out
}

func recvName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return recvName(t.X)
	case *ast.IndexExpr:
		return recvName(t.X)
	case *ast.IndexListExpr:
		return recvName(t.X)
	case *ast.Ident:
		return t.Name
	}
	return ""
}

func (d decl) String() string {
	if d.recv != "" {
		return d.recv + "." + d.name
	}
	return d.name
}

func (g *GoResolver) resolveSymbol(ctx context.Context, abs, sym string) (protocol.ResolvedRef, error) {
	e, err := g.load(ctx, abs)
	if err != nil {
		return protocol.ResolvedRef{}, err
	}
	var decls []decl
	pkgName := ""
	for _, p := range e.pkgs {
		if pkgName == "" {
			pkgName = p.Name
		}
		decls = append(decls, collectDecls(p.Syntax)...)
	}
	cands, err := parseSymbol(sym, pkgName)
	if err != nil {
		return protocol.ResolvedRef{}, err
	}
	for _, c := range cands {
		for _, d := range decls {
			if d.name == c.name && d.recv == c.recv {
				start := e.fset.Position(d.start)
				end := e.fset.Position(d.end)
				return protocol.ResolvedRef{
					Path: start.Filename, StartLine: start.Line, EndLine: end.Line,
					Symbol: sym, Kind: d.kind, Source: "symbol",
				}, nil
			}
		}
	}
	return protocol.ResolvedRef{}, fmt.Errorf("symbol %q not found in package %s%s", sym, pkgName, suggest(cands, decls))
}

func suggest(cands []symbolName, decls []decl) string {
	want := cands[0].name
	if cands[0].recv != "" {
		want = cands[0].recv + "." + cands[0].name
	}
	type s struct {
		name string
		d    int
	}
	var near []s
	seen := map[string]bool{}
	for _, d := range decls {
		n := d.String()
		if seen[n] {
			continue
		}
		seen[n] = true
		dist := levenshtein(strings.ToLower(want), strings.ToLower(n))
		if alt := levenshtein(strings.ToLower(cands[0].name), strings.ToLower(d.name)); alt < dist {
			dist = alt
		}
		if dist <= 3 {
			near = append(near, s{n, dist})
		}
	}
	if len(near) == 0 {
		return ""
	}
	sort.SliceStable(near, func(i, j int) bool { return near[i].d < near[j].d })
	if len(near) > 5 {
		near = near[:5]
	}
	names := make([]string, len(near))
	for i, n := range near {
		names[i] = n.name
	}
	return "; did you mean: " + strings.Join(names, ", ")
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
