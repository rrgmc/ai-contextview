package render

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"

	"github.com/rrgmc/ai-contextview/internal/specsrc"
)

// Op is one step of a block diff.
type Op int

const (
	OpEqual Op = iota
	OpDelete
	OpInsert
)

// BlockEdit is one block of a diff with its source text. A table is split
// into its header (the header and delimiter lines, IsHead set) and one block
// per row; Head holds the table's header lines for both.
type BlockEdit struct {
	Op     Op
	Text   string
	Head   string
	IsHead bool
}

// block is one unit of a diff: a top-level block, a list item, or a table
// header or row.
type block struct {
	text   string
	head   string
	isHead bool
}

// Blocks splits prepared markdown into the units a rendered diff compares:
// top-level blocks, with lists split into their items and tables into their
// header and rows. Each block runs from the start of its first line to the
// start of the next block, so markers and fences stay with their block.
func Blocks(src []byte) []string {
	bs := splitBlocks(src)
	out := make([]string, len(bs))
	for i, b := range bs {
		out[i] = b.text
	}
	return out
}

func splitBlocks(src []byte) []block {
	doc := md.Parser().Parse(text.NewReader(src))
	var starts []int
	tables := map[int]bool{}
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if l, ok := n.(*ast.List); ok {
			for it := l.FirstChild(); it != nil; it = it.NextSibling() {
				if s := specsrc.NodeStart(it); s >= 0 {
					starts = append(starts, lineStartOf(src, s))
				}
			}
			continue
		}
		if s := specsrc.NodeStart(n); s >= 0 {
			s = lineStartOf(src, s)
			starts = append(starts, s)
			if _, ok := n.(*extast.Table); ok {
				tables[s] = true
			}
		}
	}
	var out []block
	for i, s := range starts {
		e := len(src)
		if i+1 < len(starts) {
			e = starts[i+1]
		}
		if s >= e {
			continue
		}
		t := string(src[s:e])
		if strings.TrimSpace(t) == "" {
			continue
		}
		if tables[s] {
			out = append(out, tableBlocks(t)...)
			continue
		}
		out = append(out, block{text: t})
	}
	return out
}

// tableBlocks splits a table's source into its header (header and delimiter
// lines) and one block per row. Blank lines after a row stay with it.
func tableBlocks(t string) []block {
	lines := strings.SplitAfter(t, "\n")
	if len(lines) < 2 {
		return []block{{text: t}}
	}
	head := lines[0] + lines[1]
	out := []block{{text: head, head: head, isHead: true}}
	for _, l := range lines[2:] {
		if l == "" {
			continue
		}
		if strings.TrimSpace(l) == "" {
			out[len(out)-1].text += l
			continue
		}
		out = append(out, block{text: l, head: head})
	}
	return out
}

func lineStartOf(src []byte, off int) int {
	if off > len(src) {
		off = len(src)
	}
	return bytes.LastIndexByte(src[:off], '\n') + 1
}

// DiffBlocks compares two markdown sources block by block (longest common
// subsequence on whitespace-trimmed blocks).
func DiffBlocks(oldSrc, newSrc []byte) []BlockEdit {
	a, b := splitBlocks(oldSrc), splitBlocks(newSrc)
	ka, kb := keys(a), keys(b)
	n, m := len(a), len(b)
	// lcs[i][j] = length of the LCS of a[i:] and b[j:].
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if ka[i] == kb[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []BlockEdit
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case ka[i] == kb[j]:
			out = append(out, b[j].edit(OpEqual))
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, a[i].edit(OpDelete))
			i++
		default:
			out = append(out, b[j].edit(OpInsert))
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, a[i].edit(OpDelete))
	}
	for ; j < m; j++ {
		out = append(out, b[j].edit(OpInsert))
	}
	return out
}

func (b block) edit(op Op) BlockEdit {
	return BlockEdit{Op: op, Text: b.text, Head: b.head, IsHead: b.isHead}
}

// keys are the whitespace-normalized blocks. A table row's key includes its
// table's header, so a row matches only a row under the same header: when
// the header changes, the whole table is replaced.
func keys(blocks []block) []string {
	out := make([]string, len(blocks))
	for i, b := range blocks {
		k := norm(b.text)
		switch {
		case b.isHead:
			k = "\x00head\x00" + k
		case b.head != "":
			k = "\x00row\x00" + norm(b.head) + "\x00" + k
		}
		out[i] = k
	}
	return out
}

func norm(s string) string { return strings.Join(strings.Fields(s), " ") }

// Hunks counts the runs of changed blocks.
func Hunks(edits []BlockEdit) int {
	n, in := 0, false
	for _, e := range edits {
		if e.Op == OpEqual {
			in = false
			continue
		}
		if !in {
			n++
			in = true
		}
	}
	return n
}

// ChangesHTML renders the whole document as a block diff: unchanged blocks
// as they are, added blocks green and removed blocks red. Each run of
// neighbouring blocks with the same status is rendered in one piece. The run
// is a contiguous slice of one version of the source, so a list stays one
// list instead of one list per item.
func ChangesHTML(edits []BlockEdit) string {
	var b strings.Builder
	if Hunks(edits) == 0 {
		b.WriteString(`<p class="chg-none">No changes.</p>`)
	}
	for i := 0; i < len(edits); {
		if edits[i].IsHead {
			j := i + 1
			for j < len(edits) && !edits[j].IsHead && edits[j].Head == edits[i].Head {
				j++
			}
			b.WriteString(tableHTML(edits[i:j]))
			i = j
			continue
		}
		j := i
		var src strings.Builder
		for j < len(edits) && edits[j].Op == edits[i].Op && !edits[j].IsHead {
			src.WriteString(edits[j].Text)
			j++
		}
		html := blockHTML(src.String())
		switch edits[i].Op {
		case OpInsert:
			fmt.Fprintf(&b, `<div class="chg-add">%s</div>`, html)
		case OpDelete:
			fmt.Fprintf(&b, `<div class="chg-del">%s</div>`, html)
		default:
			b.WriteString(html)
		}
		i = j
	}
	return b.String()
}

// tableHTML renders the edits of one table: its header edit, then its rows.
// A removed or added header means the whole table was replaced, so it is
// rendered as one removed or added block. Otherwise the table renders once,
// with removed rows red and added rows green.
func tableHTML(edits []BlockEdit) string {
	var src strings.Builder
	src.WriteString(edits[0].Head)
	for _, e := range edits[1:] {
		src.WriteString(strings.TrimRight(e.Text, "\n") + "\n")
	}
	html := blockHTML(src.String())
	switch edits[0].Op {
	case OpInsert:
		return `<div class="chg-add">` + html + `</div>`
	case OpDelete:
		return `<div class="chg-del">` + html + `</div>`
	}
	body := strings.Index(html, "<tbody>")
	if body < 0 || strings.Count(html[body:], "<tr>") != len(edits)-1 {
		return html
	}
	var b strings.Builder
	b.WriteString(html[:body])
	rest := html[body:]
	for _, e := range edits[1:] {
		k := strings.Index(rest, "<tr>")
		b.WriteString(rest[:k])
		switch e.Op {
		case OpInsert:
			b.WriteString(`<tr class="chg-row-add">`)
		case OpDelete:
			b.WriteString(`<tr class="chg-row-del">`)
		default:
			b.WriteString("<tr>")
		}
		rest = rest[k+len("<tr>"):]
	}
	b.WriteString(rest)
	return b.String()
}

func blockHTML(src string) string {
	html, _, err := Markdown([]byte(src), nil, nil)
	if err != nil {
		return "<pre>" + template.HTMLEscapeString(src) + "</pre>"
	}
	return html
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// SourceChangesHTML renders the whole markdown source as a line diff.
func SourceChangesHTML(oldSrc, newSrc []byte) (string, int) {
	dmp := diffmatchpatch.New()
	ca, cb, lines := dmp.DiffLinesToChars(string(oldSrc), string(newSrc))
	diffs := dmp.DiffCharsToLines(dmp.DiffMain(ca, cb, false), lines)

	var b strings.Builder
	hunks, in := 0, false
	b.WriteString(`<pre class="src-diff">`)
	for _, d := range diffs {
		class, sign := "sd-ctx", " "
		switch d.Type {
		case diffmatchpatch.DiffInsert:
			class, sign = "sd-add", "+"
		case diffmatchpatch.DiffDelete:
			class, sign = "sd-del", "-"
		}
		if class == "sd-ctx" {
			in = false
		} else if !in {
			hunks++
			in = true
		}
		for _, l := range strings.SplitAfter(d.Text, "\n") {
			if l == "" {
				continue
			}
			// Each line is a block-level span, so no newline between them.
			fmt.Fprintf(&b, `<span class="%s">%s %s</span>`, class, sign, template.HTMLEscapeString(strings.TrimSuffix(l, "\n")))
		}
	}
	b.WriteString("</pre>")
	if hunks == 0 {
		return `<p class="chg-none">No changes.</p>` + b.String(), 0
	}
	return b.String(), hunks
}
