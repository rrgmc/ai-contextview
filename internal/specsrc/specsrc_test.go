package specsrc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rrgmc/ai-specview/internal/protocol"
)

type quoteFixture struct {
	Name    string `json:"name"`
	Source  string `json:"source"`
	Quote   string `json:"quote"`
	Section string `json:"section"`
	Match   string `json:"match"`
	Expect  string `json:"expect"`
}

// LoadQuoteFixtures reads protocol/quotes/quotes.json.
func loadQuoteFixtures(t *testing.T) []quoteFixture {
	data, err := os.ReadFile("../../protocol/quotes/quotes.json")
	require.NoError(t, err)
	var fx []quoteFixture
	require.NoError(t, json.Unmarshal(data, &fx))
	require.NotEmpty(t, fx)
	return fx
}

func TestQuoteFixtures(t *testing.T) {
	for _, f := range loadQuoteFixtures(t) {
		t.Run(f.Name, func(t *testing.T) {
			r := Match([]byte(f.Source), f.Quote, f.Section)
			assert.Equal(t, protocol.Match(f.Match), r.Quality)
			if f.Expect != "" {
				assert.Equal(t, f.Expect, f.Source[r.Start:r.End])
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"Hello **World**":            "hello world",
		"a  \n\t b":                  "a b",
		"see [the docs](http://x/y)": "see the docs",
		"![alt text](img.png) after": "alt text after",
		"## Heading\n\n- item one":   "heading item one",
		"1. first\n2. second":        "first second",
		"> quoted\n> more":           "quoted more",
		"x <b>bold</b> y":            "x bold y",
		"“hi” – there":               `"hi" - there`,
		"| a | b |":                  "a b",
		"snake\\_case":               "snakecase",
	}
	for in, want := range cases {
		assert.Equal(t, want, Normalize(in).Text, "input %q", in)
	}
}

func TestNormalizeIndexMap(t *testing.T) {
	src := "Some **bold** and `code` text."
	n := Normalize(src)
	require.Len(t, n.Start, len(n.Text))
	for i := 0; i < len(n.Text); i++ {
		a, b := n.Start[i], n.End[i]
		require.True(t, a < b && b <= len(src))
	}
	i := len("some ")
	a, b := n.SourceRange(i, i+len("bold and code"))
	assert.Equal(t, "bold** and `code", src[a:b])
}

func TestSections(t *testing.T) {
	src := []byte("# Spec\n\nIntro\n\n## Retry policy\n\nText\n\n### Caps\n\nMore\n\n## Errors\n\nE\n")
	secs := Sections(src)
	require.Len(t, secs, 4)
	assert.Equal(t, []string{"Spec", "Retry policy", "Caps", "Errors"}, Headings(secs))
	assert.Equal(t, "spec--retry-policy--caps", secs[2].ID)
	assert.Equal(t, "## Retry policy\n\nText\n\n### Caps\n\nMore\n\n", string(src[secs[1].Start:secs[1].End]))
	assert.Equal(t, "## Retry policy\n\nText\n\n", string(src[secs[1].Start:secs[1].BodyEnd]))
	assert.NotNil(t, FindSection(secs, "retry POLICY"))
	assert.NotNil(t, FindSection(secs, "Retry policy > Caps"))
}

func TestPrepareFrontmatter(t *testing.T) {
	src := []byte("---\nkind: adr\ntitle: x\n---\n# Decision\n\nWe cap.\n")
	body, fm := Prepare(src)
	assert.Len(t, body, len(src))
	assert.Contains(t, fm, "kind: adr")
	assert.Equal(t, []string{"Decision"}, Headings(Sections(body)))
	assert.Equal(t, protocol.KindADR, InferKind("x.md", fm))
}

func TestInferKind(t *testing.T) {
	assert.Equal(t, protocol.KindADR, InferKind("adr/002-cap.md", ""))
	assert.Equal(t, protocol.KindADR, InferKind("decisions/1.md", ""))
	assert.Equal(t, protocol.KindResearch, InferKind("research/bench.md", ""))
	assert.Equal(t, protocol.KindOpenQuestion, InferKind("open-questions/q.md", ""))
	assert.Equal(t, protocol.KindSpec, InferKind("specs/retry/spec.md", ""))
	assert.Equal(t, protocol.KindSpec, InferKind("proposal.md", ""))
	assert.Equal(t, protocol.KindOther, InferKind("notes.md", ""))
	assert.Equal(t, protocol.KindResearch, InferKind("adr/x.md", "kind: research"))
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	for p, c := range files {
		full := filepath.Join(dir, p)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(c), 0o644))
	}
}

func TestLoadRootFolder(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"a-notes.md":              "# Notes\n",
		"proposal.md":             "# Overview\n\n## Retry policy\n",
		"adr/002-cap.md":          "# Context\n\n# Decision\n",
		".hidden/x.md":            "# Hidden\n",
		"a/b/c/too-deep.md":       "# Deep\n",
		"research/bench.markdown": "# Bench\n",
		"open-questions/q-cap.md": "# Q\n",
		"not-markdown.txt":        "x",
	})
	r, err := LoadRoot(dir, "")
	require.NoError(t, err)
	assert.Equal(t, "proposal.md", r.Primary)
	assert.Equal(t, "proposal.md", r.Artifacts[0].Path)
	var paths []string
	for _, a := range r.Artifacts {
		paths = append(paths, a.Path)
	}
	assert.ElementsMatch(t, []string{"proposal.md", "a-notes.md", "adr/002-cap.md", "research/bench.markdown", "open-questions/q-cap.md"}, paths)
	assert.Equal(t, protocol.KindADR, r.Artifact("adr/002-cap.md").Kind)
	assert.Equal(t, []string{"Context", "Decision"}, r.Artifact("adr/002-cap.md").Sections)

	r, err = LoadRoot(dir, "adr/002-cap.md")
	require.NoError(t, err)
	assert.Equal(t, "adr/002-cap.md", r.Primary)

	_, err = LoadRoot(dir, "missing.md")
	assert.Equal(t, protocol.CodeBadRequest, Code(err))
}

func TestLoadRootErrors(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"x.txt": "x", "empty/y.txt": "y", "one.md": "# One\n"})

	_, err := LoadRoot(filepath.Join(dir, "nope.md"), "")
	assert.Equal(t, protocol.CodeFileNotFound, Code(err))
	_, err = LoadRoot(filepath.Join(dir, "x.txt"), "")
	assert.Equal(t, protocol.CodeNotMarkdown, Code(err))
	_, err = LoadRoot(filepath.Join(dir, "empty"), "")
	assert.Equal(t, protocol.CodeEmptyFolder, Code(err))

	r, err := LoadRoot(filepath.Join(dir, "one.md"), "")
	require.NoError(t, err)
	assert.False(t, r.IsDir)
	assert.Equal(t, "one.md", r.Primary)
	assert.Equal(t, dir, r.Dir)
}

func TestClosestHeadings(t *testing.T) {
	src := []byte("# Overview\n\nGeneral intro.\n\n## Retry policy\n\nBackoff grows exponentially up to a cap.\n\n## Storage\n\nRows are kept in Postgres.\n")
	got := ClosestHeadings(src, "exponential backoff cap", 3)
	require.NotEmpty(t, got)
	assert.Equal(t, "Retry policy", got[0])
}
