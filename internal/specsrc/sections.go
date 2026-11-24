package specsrc

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

// Section is one heading of a markdown document.
type Section struct {
	Level int
	Text  string
	// Path is the heading texts from the top level down to this heading.
	Path []string
	// ID is the HTML id derived from the path ("design--retry-policy").
	ID string
	// Start is the byte offset of the heading line.
	Start int
	// BodyEnd is where the next heading (any level) starts.
	BodyEnd int
	// End is where the next heading of the same or a higher level starts.
	End int
}

// Hash returns a hash of the section's own content (heading to next heading).
func (s *Section) Hash(src []byte) string {
	h := sha256.Sum256(bytes.TrimRight(src[s.Start:s.BodyEnd], " \t\r\n"))
	return hex.EncodeToString(h[:8])
}

// Markdown returns the goldmark instance used everywhere, so the section index,
// the bridge pre-check and the daemon rendering agree on the AST.
func Markdown() goldmark.Markdown {
	return goldmark.New(goldmark.WithExtensions(extension.GFM))
}

// Prepare blanks a leading YAML frontmatter block (keeping byte offsets) and
// returns the frontmatter text separately.
func Prepare(src []byte) (body []byte, frontmatter string) {
	if !bytes.HasPrefix(src, []byte("---\n")) && !bytes.HasPrefix(src, []byte("---\r\n")) {
		return src, ""
	}
	rest := src[4:]
	end := -1
	for off := 0; off < len(rest); {
		nl := bytes.IndexByte(rest[off:], '\n')
		line := rest[off:]
		if nl >= 0 {
			line = rest[off : off+nl]
		}
		if t := strings.TrimRight(string(line), "\r"); t == "---" || t == "..." {
			end = 4 + off + len(line)
			break
		}
		if nl < 0 {
			break
		}
		off += nl + 1
	}
	if end < 0 {
		return src, ""
	}
	out := make([]byte, len(src))
	copy(out, src)
	for i := 0; i < end; i++ {
		if out[i] != '\n' {
			out[i] = ' '
		}
	}
	return out, string(src[4:end])
}

// Parse parses prepared markdown source.
func Parse(src []byte) ast.Node {
	return Markdown().Parser().Parse(text.NewReader(src))
}

// Sections returns the heading index of prepared markdown source.
func Sections(src []byte) []Section {
	return SectionsOf(Parse(src), src)
}

// SectionsOf builds the heading index from a parsed document.
func SectionsOf(doc ast.Node, src []byte) []Section {
	var out []Section
	var stack []string
	seen := map[string]int{}
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		h, ok := n.(*ast.Heading)
		if !ok {
			continue
		}
		txt := strings.TrimSpace(NodeText(h, src))
		start := lineStart(src, nodeStart(h))
		if len(stack) >= h.Level {
			stack = stack[:h.Level-1]
		}
		for len(stack) < h.Level-1 {
			stack = append(stack, "")
		}
		stack = append(stack, txt)
		path := make([]string, 0, len(stack))
		for _, p := range stack {
			if p != "" {
				path = append(path, p)
			}
		}
		id := pathID(path)
		if c := seen[id]; c > 0 {
			seen[id] = c + 1
			id = id + "-" + itoa(c+1)
		} else {
			seen[id] = 1
		}
		out = append(out, Section{Level: h.Level, Text: txt, Path: path, ID: id, Start: start})
	}
	for i := range out {
		out[i].BodyEnd = len(src)
		out[i].End = len(src)
		if i+1 < len(out) {
			out[i].BodyEnd = out[i+1].Start
		}
		for j := i + 1; j < len(out); j++ {
			if out[j].Level <= out[i].Level {
				out[i].End = out[j].Start
				break
			}
		}
	}
	return out
}

// Headings returns the heading texts of a section index.
func Headings(secs []Section) []string {
	out := make([]string, len(secs))
	for i, s := range secs {
		out[i] = s.Text
	}
	return out
}

// FindSection finds a section by heading text (normalized comparison), or by
// a "A > B" / "A / B" path suffix.
func FindSection(secs []Section, name string) *Section {
	want := NormalizeQuote(name)
	for i := range secs {
		if NormalizeQuote(secs[i].Text) == want {
			return &secs[i]
		}
	}
	for i := range secs {
		for _, sep := range []string{" > ", " / "} {
			full := NormalizeQuote(strings.Join(secs[i].Path, sep))
			if full == want || strings.HasSuffix(full, NormalizeQuote(sep)+want) {
				return &secs[i]
			}
		}
	}
	return nil
}

// SectionAt returns the innermost section containing a byte offset.
func SectionAt(secs []Section, off int) *Section {
	var best *Section
	for i := range secs {
		if secs[i].Start <= off && off < secs[i].BodyEnd {
			best = &secs[i]
		}
	}
	return best
}

// NodeText collects the visible text of an inline subtree.
func NodeText(n ast.Node, src []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := c.(type) {
		case *ast.Text:
			b.Write(t.Segment.Value(src))
			if t.SoftLineBreak() || t.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(t.Value)
		case *ast.AutoLink:
			b.Write(t.Label(src))
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

// nodeStart returns the first source byte of a node, looking into children
// for container blocks.
func nodeStart(n ast.Node) int {
	if n.Type() == ast.TypeBlock {
		if l := n.Lines(); l != nil && l.Len() > 0 {
			return l.At(0).Start
		}
	}
	if t, ok := n.(*ast.Text); ok {
		return t.Segment.Start
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if s := nodeStart(c); s >= 0 {
			return s
		}
	}
	return -1
}

// NodeStart is the exported form of nodeStart.
func NodeStart(n ast.Node) int { return nodeStart(n) }

func lineStart(src []byte, off int) int {
	if off < 0 {
		return 0
	}
	if off > len(src) {
		off = len(src)
	}
	i := bytes.LastIndexByte(src[:off], '\n')
	return i + 1
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Slug turns a heading text into an id fragment.
func Slug(s string) string {
	s = nonSlug.ReplaceAllString(strings.ToLower(s), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "section"
	}
	return s
}

func pathID(path []string) string {
	parts := make([]string, len(path))
	for i, p := range path {
		parts[i] = Slug(p)
	}
	return strings.Join(parts, "--")
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	return string(b[p:])
}
