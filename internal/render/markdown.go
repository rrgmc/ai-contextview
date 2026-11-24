// Package render turns markdown and code into HTML for the viewer.
package render

import (
	"bytes"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/rrgmc/ai-specview/internal/specsrc"
)

// Mark is a source range to highlight. Heading marks tint whole blocks.
type Mark struct {
	Start, End int
	Ref        string
	Heading    bool
}

// KindHighlight is the inline node that wraps highlighted text.
var KindHighlight = ast.NewNodeKind("Highlight")

// Highlight wraps the inline content of a quote match.
type Highlight struct {
	ast.BaseInline
	Ref string
}

// Kind implements ast.Node.
func (n *Highlight) Kind() ast.NodeKind { return KindHighlight }

// Dump implements ast.Node.
func (n *Highlight) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, map[string]string{"Ref": n.Ref}, nil)
}

type highlightRenderer struct{}

func (highlightRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindHighlight, func(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			_, _ = w.WriteString(`<mark class="q" data-ref="`)
			_, _ = w.Write(util.EscapeHTML([]byte(n.(*Highlight).Ref)))
			_, _ = w.WriteString(`">`)
		} else {
			_, _ = w.WriteString("</mark>")
		}
		return ast.WalkContinue, nil
	})
}

var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(renderer.WithNodeRenderers(util.Prioritized(highlightRenderer{}, 500))),
)

// Markdown renders prepared markdown source. Headings get ids from their
// path; headings whose id is in changed get the "changed" class; marks are
// wrapped in <mark> elements (inline) or tint their blocks (heading marks).
func Markdown(src []byte, marks []Mark, changed map[string]bool) (string, []specsrc.Section, error) {
	doc := md.Parser().Parse(text.NewReader(src))
	secs := specsrc.SectionsOf(doc, src)

	i := 0
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if _, ok := n.(*ast.Heading); ok && i < len(secs) {
			n.SetAttributeString("id", []byte(secs[i].ID))
			if changed[secs[i].ID] {
				addClass(n, "changed")
			}
			i++
		}
	}

	var inline []Mark
	for _, m := range marks {
		if m.End <= m.Start {
			continue
		}
		if m.Heading {
			for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
				if s := specsrc.NodeStart(n); s >= m.Start && s < m.End {
					addClass(n, "q-section")
					n.SetAttributeString("data-ref", []byte(m.Ref))
				}
			}
			continue
		}
		inline = append(inline, m)
	}
	if len(inline) > 0 {
		applyMarks(doc, inline)
	}

	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, src, doc); err != nil {
		return "", nil, err
	}
	return buf.String(), secs, nil
}

func addClass(n ast.Node, class string) {
	if v, ok := n.AttributeString("class"); ok {
		if b, ok := v.([]byte); ok {
			n.SetAttributeString("class", []byte(string(b)+" "+class))
			return
		}
	}
	n.SetAttributeString("class", []byte(class))
}

func overlaps(a, b, start, end int) bool { return a < end && start < b }

// applyMarks splits text nodes at mark boundaries and wraps covered pieces in
// Highlight nodes. Code spans are wrapped whole, because their renderer only
// accepts text children. A range across paragraphs yields several <mark>s
// with the same data-ref.
func applyMarks(doc ast.Node, marks []Mark) {
	var texts []*ast.Text
	var spans []*ast.CodeSpan
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := n.(type) {
		case *ast.CodeSpan:
			spans = append(spans, t)
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			texts = append(texts, t)
		}
		return ast.WalkContinue, nil
	})

	for _, cs := range spans {
		a, b := spanRange(cs)
		for _, m := range marks {
			if overlaps(a, b, m.Start, m.End) {
				parent := cs.Parent()
				h := &Highlight{Ref: m.Ref}
				parent.ReplaceChild(parent, cs, h)
				h.AppendChild(h, cs)
				break
			}
		}
	}

	for _, t := range texts {
		a, b := t.Segment.Start, t.Segment.Stop
		cuts := []int{a, b}
		hit := false
		for _, m := range marks {
			if !overlaps(a, b, m.Start, m.End) {
				continue
			}
			hit = true
			if m.Start > a && m.Start < b {
				cuts = append(cuts, m.Start)
			}
			if m.End > a && m.End < b {
				cuts = append(cuts, m.End)
			}
		}
		if !hit {
			continue
		}
		sort.Ints(cuts)
		parent := t.Parent()
		var nodes []ast.Node
		for k := 0; k+1 < len(cuts); k++ {
			s, e := cuts[k], cuts[k+1]
			if s == e {
				continue
			}
			seg := t.Segment
			seg.Start, seg.Stop = s, e
			piece := ast.NewTextSegment(seg)
			piece.SetRaw(t.IsRaw())
			var node ast.Node = piece
			for _, m := range marks {
				if overlaps(s, e, m.Start, m.End) {
					h := &Highlight{Ref: m.Ref}
					h.AppendChild(h, piece)
					node = h
					break
				}
			}
			nodes = append(nodes, node)
			if k+2 == len(cuts) {
				piece.SetSoftLineBreak(t.SoftLineBreak())
				piece.SetHardLineBreak(t.HardLineBreak())
			}
		}
		for _, n := range nodes {
			parent.InsertBefore(parent, t, n)
		}
		parent.RemoveChild(parent, t)
	}
	mergeAdjacent(doc)
}

// mergeAdjacent joins sibling Highlight nodes with the same ref, so a quote
// that goldmark split into several text nodes yields one <mark>.
func mergeAdjacent(doc ast.Node) {
	var hs []*Highlight
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if h, ok := n.(*Highlight); ok && entering {
			hs = append(hs, h)
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	for _, h := range hs {
		if h.Parent() == nil {
			continue
		}
		for {
			next, ok := h.NextSibling().(*Highlight)
			if !ok || next.Ref != h.Ref {
				break
			}
			for c := next.FirstChild(); c != nil; {
				nc := c.NextSibling()
				next.RemoveChild(next, c)
				h.AppendChild(h, c)
				c = nc
			}
			next.Parent().RemoveChild(next.Parent(), next)
		}
	}
}

func spanRange(cs *ast.CodeSpan) (int, int) {
	a, b := -1, -1
	for c := cs.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			if a < 0 {
				a = t.Segment.Start
			}
			b = t.Segment.Stop
		}
	}
	return a, b
}

// SectionChanges compares section hashes. It returns the ids of sections that
// are new or changed in cur compared to prev.
func SectionChanges(prev, cur map[string]string) []string {
	var out []string
	for id, h := range cur {
		if prev[id] != h {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// SectionHashes hashes every section's own content by id.
func SectionHashes(src []byte, secs []specsrc.Section) map[string]string {
	out := make(map[string]string, len(secs))
	for i := range secs {
		out[secs[i].ID] = secs[i].Hash(src)
	}
	return out
}

// SectionIDs maps heading texts to ids (first match wins).
func SectionIDs(secs []specsrc.Section, names []string) []string {
	var out []string
	for _, n := range names {
		if s := specsrc.FindSection(secs, n); s != nil {
			out = append(out, s.ID)
		}
	}
	return out
}

// Plain is a helper for tests: markdown text without tags.
func Plain(html string) string {
	var b strings.Builder
	in := false
	for _, r := range html {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}
