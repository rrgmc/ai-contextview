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
