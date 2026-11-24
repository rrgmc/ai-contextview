package render

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"github.com/rrgmc/ai-specview/internal/specsrc"
)

// Op is one step of a block diff.
type Op int

const (
	OpEqual Op = iota
	OpDelete
	OpInsert
)

// BlockEdit is one block of a diff with its source text.
type BlockEdit struct {
	Op   Op
	Text string
}

// Blocks splits prepared markdown into the units a rendered diff compares:
// top-level blocks, with lists split into their items. Each block runs from
// the start of its first line to the start of the next block, so markers and
// fences stay with their block.
func Blocks(src []byte) []string {
	doc := md.Parser().Parse(text.NewReader(src))
	var starts []int
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
			starts = append(starts, lineStartOf(src, s))
		}
	}
	var out []string
	for i, s := range starts {
		e := len(src)
		if i+1 < len(starts) {
			e = starts[i+1]
		}
		if s >= e {
			continue
		}
		if b := strings.TrimSpace(string(src[s:e])); b != "" {
			out = append(out, string(src[s:e]))
		}
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
	a, b := Blocks(oldSrc), Blocks(newSrc)
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
			out = append(out, BlockEdit{OpEqual, b[j]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, BlockEdit{OpDelete, a[i]})
			i++
		default:
			out = append(out, BlockEdit{OpInsert, b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, BlockEdit{OpDelete, a[i]})
	}
	for ; j < m; j++ {
		out = append(out, BlockEdit{OpInsert, b[j]})
	}
	return out
}

func keys(blocks []string) []string {
	out := make([]string, len(blocks))
	for i, b := range blocks {
		out[i] = strings.Join(strings.Fields(b), " ")
	}
	return out
}

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
		j := i
		var src strings.Builder
		for j < len(edits) && edits[j].Op == edits[i].Op {
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
