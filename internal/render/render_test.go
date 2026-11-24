package render

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rrgmc/ai-specview/internal/specsrc"
)

var markRe = regexp.MustCompile(`(?s)<mark class="q" data-ref="r">(.*?)</mark>`)

// marked returns the plain text inside every <mark data-ref="r">.
func marked(html string) string {
	var parts []string
	for _, m := range markRe.FindAllStringSubmatch(html, -1) {
		parts = append(parts, Plain(m[1]))
	}
	return strings.Join(parts, "|")
}

func renderQuote(t *testing.T, src, quote, section string) string {
	b := []byte(src)
	m := specsrc.Match(b, quote, section)
	require.NotEqual(t, "none", string(m.Quality))
	html, _, err := Markdown(b, []Mark{{Start: m.Start, End: m.End, Ref: "r", Heading: m.Quality == "heading"}}, nil)
	require.NoError(t, err)
	return html
}

func TestHighlightInlineFormatting(t *testing.T) {
	src := "# Spec\n\nRetries use **exponential** backoff via `Retrier.Next`, see [the RFC](https://x/rfc) now.\n"
	html := renderQuote(t, src, "exponential backoff via Retrier.Next, see the RFC", "")
	assert.Equal(t, "exponential| backoff via Retrier.Next, see |the RFC", marked(html))
	assert.Contains(t, html, `<strong><mark class="q" data-ref="r">exponential</mark></strong>`)
	assert.Contains(t, html, `<mark class="q" data-ref="r"> backoff via <code>Retrier.Next</code>, see </mark>`)
	assert.Contains(t, html, `<a href="https://x/rfc"><mark class="q" data-ref="r">the RFC</mark></a>`)
	assert.Contains(t, html, " now.")
}

func TestHighlightLineBreakAndParagraphs(t *testing.T) {
	src := "# D\n\nThe client keeps a budget\nof retries per host.\n\nSecond paragraph here.\n"
	html := renderQuote(t, src, "keeps a budget of retries", "")
	assert.Equal(t, "keeps a budget\nof retries", marked(html))

	html = renderQuote(t, src, "per host. Second paragraph", "")
	assert.Equal(t, "per host.|Second paragraph", marked(html))
	assert.Equal(t, 2, strings.Count(html, `data-ref="r"`))
}

func TestHighlightPartialWord(t *testing.T) {
	src := "Alpha beta gamma.\n"
	html := renderQuote(t, src, "beta", "")
	assert.Contains(t, html, `<p>Alpha <mark class="q" data-ref="r">beta</mark> gamma.</p>`)
}

func TestHeadingFallback(t *testing.T) {
	src := "# A\n\nx\n\n## Retry policy\n\nBody text.\n\n- item\n\n## Other\n\ny\n"
	html := renderQuote(t, src, "zzzz not here at all zzzz", "Retry policy")
	assert.Contains(t, html, `<h2 id="a--retry-policy" class="q-section" data-ref="r">`)
	assert.Contains(t, html, `<p class="q-section" data-ref="r">Body text.</p>`)
	assert.Contains(t, html, `<ul class="q-section" data-ref="r">`)
	assert.Contains(t, html, `<p>y</p>`)
}

func TestHeadingIDsAndChanged(t *testing.T) {
	src := []byte("# Spec\n\n## Retry policy\n\nA\n\n## Retry policy\n\nB\n")
	html, secs, err := Markdown(src, nil, map[string]bool{"spec--retry-policy": true})
	require.NoError(t, err)
	require.Len(t, secs, 3)
	assert.Contains(t, html, `<h2 id="spec--retry-policy" class="changed">`)
	assert.Contains(t, html, `<h2 id="spec--retry-policy-2">`)
}

func TestSafeHTML(t *testing.T) {
	html, _, err := Markdown([]byte("hi <script>alert(1)</script>\n"), nil, nil)
	require.NoError(t, err)
	assert.NotContains(t, html, "<script>")
}

func TestSectionChanges(t *testing.T) {
	v1 := []byte("# A\n\none\n\n## B\n\ntwo\n\n## C\n\nthree\n")
	v2 := []byte("# A\n\none\n\n## B\n\ntwo changed\n\n## C\n\nthree\n\n## D\n\nnew\n")
	s1, s2 := specsrc.Sections(v1), specsrc.Sections(v2)
	got := SectionChanges(SectionHashes(v1, s1), SectionHashes(v2, s2))
	assert.Equal(t, []string{"a--b", "a--d"}, got)
}

func TestCode(t *testing.T) {
	src := []byte("package x\n\nfunc A() {\n\treturn\n}\n")
	html, err := Code("x.go", src, 3, 5)
	require.NoError(t, err)
	assert.Contains(t, html, `id="L3"`)
	assert.Contains(t, html, `class="line hl"`)
	assert.Equal(t, 5, LineCount(src))
	css := CodeCSS()
	assert.Contains(t, css, "prefers-color-scheme: dark")
	assert.Contains(t, css, `:root[data-theme="dark"] .chroma`)
	// github colors names and punctuation #1f2328; github-dark leaves them
	// out, so the dark theme must not keep the light color.
	assert.Contains(t, css, `:root[data-theme="dark"] .chroma .nx { color: inherit }`)
	assert.Contains(t, css, `:root:not([data-theme="light"]) .chroma .p { color: inherit }`)
}
