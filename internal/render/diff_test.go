package render

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const diffOld = `# Spec

## Retry policy

Retries use exponential backoff.

- one
- two

## Errors

Errors are wrapped.

## Rollout

Behind a flag.
`

func TestBlocks(t *testing.T) {
	blocks := Blocks([]byte(diffOld))
	require.Len(t, blocks, 9) // h1, h2, p, 2 list items, h2, p, h2, p
	assert.Equal(t, "- one\n", blocks[3])
	assert.Equal(t, "- two\n\n", blocks[4])
}

func TestDiffBlocksParagraphChange(t *testing.T) {
	newSrc := strings.Replace(diffOld, "Errors are wrapped.", "Errors are wrapped with context.", 1)
	edits := DiffBlocks([]byte(diffOld), []byte(newSrc))
	assert.Equal(t, 1, Hunks(edits))

	var del, ins []string
	for _, e := range edits {
		switch e.Op {
		case OpDelete:
			del = append(del, strings.TrimSpace(e.Text))
		case OpInsert:
			ins = append(ins, strings.TrimSpace(e.Text))
		}
	}
	assert.Equal(t, []string{"Errors are wrapped."}, del)
	assert.Equal(t, []string{"Errors are wrapped with context."}, ins)

	html := ChangesHTML(edits)
	assert.Contains(t, html, `<div class="chg-del"><p>Errors are wrapped.</p>`)
	assert.Contains(t, html, `<div class="chg-add"><p>Errors are wrapped with context.</p>`)
	// The whole document is shown: unchanged blocks render as they are.
	assert.Contains(t, html, `<h1 id="spec">Spec</h1>`)
	assert.Contains(t, html, `<p>Behind a flag.</p>`)
	assert.NotContains(t, html, "unchanged")
}

func TestDiffBlocksListItemAndWhitespace(t *testing.T) {
	newSrc := strings.Replace(diffOld, "- two\n", "- two\n- three\n", 1)
	newSrc = strings.Replace(newSrc, "Behind a flag.", "Behind   a\nflag.", 1) // reflow only
	edits := DiffBlocks([]byte(diffOld), []byte(newSrc))
	assert.Equal(t, 1, Hunks(edits))
	html := ChangesHTML(edits)
	assert.Contains(t, html, `<div class="chg-add"><ul>
<li>three</li>`)
}

func TestUnchangedListStaysOneList(t *testing.T) {
	newSrc := strings.Replace(diffOld, "Behind a flag.", "Behind a flag, off by default.", 1)
	html := ChangesHTML(DiffBlocks([]byte(diffOld), []byte(newSrc)))
	assert.Contains(t, html, "<ul>\n<li>one</li>\n<li>two</li>\n</ul>")
	assert.Equal(t, 1, strings.Count(html, "<ul>"))
}

func TestNoChanges(t *testing.T) {
	edits := DiffBlocks([]byte(diffOld), []byte(diffOld))
	assert.Equal(t, 0, Hunks(edits))
	assert.Contains(t, ChangesHTML(edits), "No changes.")
	assert.Contains(t, ChangesHTML(edits), "<p>Behind a flag.</p>", "the document is still shown")
	html, n := SourceChangesHTML([]byte(diffOld), []byte(diffOld))
	assert.Equal(t, 0, n)
	assert.Contains(t, html, "No changes.")
}

func TestNewFile(t *testing.T) {
	edits := DiffBlocks(nil, []byte(diffOld))
	assert.Equal(t, 1, Hunks(edits))
	for _, e := range edits {
		assert.Equal(t, OpInsert, e.Op)
	}
}

func TestSourceChanges(t *testing.T) {
	newSrc := strings.Replace(diffOld, "Behind a flag.", "Behind a <flag>.", 1)
	html, n := SourceChangesHTML([]byte(diffOld), []byte(newSrc))
	assert.Equal(t, 1, n)
	assert.Contains(t, html, `<span class="sd-del">- Behind a flag.</span>`)
	assert.Contains(t, html, `<span class="sd-add">+ Behind a &lt;flag&gt;.</span>`)
	assert.Contains(t, html, `<span class="sd-ctx">  # Spec</span>`, "the whole source is shown")
}

const tableOld = `# Spec

| Interface | Kind |
|---|---|
| POST /a | route |
| PUT /b | route |
| flag x | flag |

After.
`

func TestBlocksSplitsTables(t *testing.T) {
	blocks := Blocks([]byte(tableOld))
	assert.Equal(t, []string{
		"# Spec\n\n",
		"| Interface | Kind |\n|---|---|\n",
		"| POST /a | route |\n",
		"| PUT /b | route |\n",
		"| flag x | flag |\n\n",
		"After.\n",
	}, blocks)
}

func TestDiffTableCellChange(t *testing.T) {
	newSrc := strings.Replace(tableOld, "| PUT /b | route |", "| PUT /b | HTTP route |", 1)
	edits := DiffBlocks([]byte(tableOld), []byte(newSrc))
	assert.Equal(t, 1, Hunks(edits))

	html := ChangesHTML(edits)
	assert.Equal(t, 1, strings.Count(html, "<table>"), "the table renders once")
	assert.NotContains(t, html, `class="chg-del"`)
	assert.NotContains(t, html, `class="chg-add"`)
	assert.Contains(t, html, "<tr>\n<td>POST /a</td>")
	assert.Contains(t, html, `<tr class="chg-row-del">`+"\n<td>PUT /b</td>\n<td>route</td>")
	assert.Contains(t, html, `<tr class="chg-row-add">`+"\n<td>PUT /b</td>\n<td>HTTP route</td>")
	assert.Contains(t, html, "<tr>\n<td>flag x</td>")
	assert.Less(t, strings.Index(html, "chg-row-del"), strings.Index(html, "chg-row-add"), "the old row comes first")
	assert.Contains(t, html, "<p>After.</p>")
}

func TestDiffTableRowAddedAndRemoved(t *testing.T) {
	newSrc := strings.Replace(tableOld, "| POST /a | route |\n", "", 1)
	newSrc = strings.Replace(newSrc, "| flag x | flag |\n", "| flag x | flag |\n| DELETE /c | route |\n", 1)
	html := ChangesHTML(DiffBlocks([]byte(tableOld), []byte(newSrc)))
	assert.Equal(t, 1, strings.Count(html, "<table>"))
	assert.Contains(t, html, `<tr class="chg-row-del">`+"\n<td>POST /a</td>")
	assert.Contains(t, html, `<tr class="chg-row-add">`+"\n<td>DELETE /c</td>")
	assert.Equal(t, 2, strings.Count(html, "<tr>\n<td>"), "PUT /b and flag x are unchanged")
}

func TestDiffTableHeaderChange(t *testing.T) {
	newSrc := strings.Replace(tableOld, "| Interface | Kind |\n|---|---|\n", "| Interface | Kind | Consumers |\n|---|---|---|\n", 1)
	newSrc = strings.Replace(newSrc, "| POST /a | route |", "| POST /a | route | portal |", 1)
	edits := DiffBlocks([]byte(tableOld), []byte(newSrc))
	html := ChangesHTML(edits)
	// The rows no longer line up: the old table is removed and the new one
	// added, each whole.
	assert.Equal(t, 2, strings.Count(html, "<table>"))
	assert.Contains(t, html, `<div class="chg-del"><table>`)
	assert.Contains(t, html, `<div class="chg-add"><table>`)
	assert.NotContains(t, html, "chg-row-")
}

func TestDiffTableAdded(t *testing.T) {
	newSrc := strings.Replace(tableOld, "After.\n", "After.\n\n| A | B |\n|---|---|\n| 1 | 2 |\n", 1)
	html := ChangesHTML(DiffBlocks([]byte(tableOld), []byte(newSrc)))
	assert.Contains(t, html, `<div class="chg-add"><table>`)
	assert.Contains(t, html, "<td>1</td>")
	assert.Equal(t, 1, Hunks(DiffBlocks([]byte(tableOld), []byte(newSrc))))
}

func TestDiffTableUnchanged(t *testing.T) {
	html := ChangesHTML(DiffBlocks([]byte(tableOld), []byte(tableOld)))
	assert.Contains(t, html, "No changes.")
	assert.Equal(t, 1, strings.Count(html, "<table>"))
	assert.NotContains(t, html, "chg-row-")
}
