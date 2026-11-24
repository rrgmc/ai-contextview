package specsrc

import (
	"sort"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"

	"github.com/rrgmc/ai-specview/internal/protocol"
)

// Result is a quote match: a source byte range and its quality.
type Result struct {
	Quality protocol.Match
	Start   int
	End     int
}

// Found reports whether the quote itself was located (not only a heading).
func (r Result) Found() bool {
	return r.Quality == protocol.MatchExact || r.Quality == protocol.MatchNormalized || r.Quality == protocol.MatchFuzzy
}

const anchorLen = 32

// Match locates a quote in prepared markdown source. Stages: exact →
// normalized → fuzzy → heading (when section is set) → none.
func Match(src []byte, quote, section string) Result {
	s := string(src)
	q := strings.TrimSpace(quote)
	if q != "" {
		if i := strings.Index(s, q); i >= 0 {
			return Result{protocol.MatchExact, i, i + len(q)}
		}
		n := Normalize(s)
		nq := NormalizeQuote(q)
		if nq != "" {
			if i := strings.Index(n.Text, nq); i >= 0 {
				a, b := n.SourceRange(i, i+len(nq))
				return Result{protocol.MatchNormalized, a, b}
			}
			if i, j, ok := fuzzy(n.Text, nq); ok {
				a, b := n.SourceRange(i, j)
				return Result{protocol.MatchFuzzy, a, b}
			}
		}
	}
	if section != "" {
		if sec := FindSection(Sections(src), section); sec != nil {
			return Result{protocol.MatchHeading, sec.Start, sec.End}
		}
	}
	return Result{Quality: protocol.MatchNone}
}

// fuzzy finds an approximate match of the normalized quote q in text. It
// locates the first and last 32 characters with bitap, takes the span between
// them, and accepts it when its length is within ±30% of the quote's and its
// edit distance is at most 30% of the quote's length.
func fuzzy(text, q string) (int, int, bool) {
	if len(q) < 12 || len(text) == 0 {
		return 0, 0, false
	}
	dmp := diffmatchpatch.New()
	dmp.MatchThreshold = 0.4
	dmp.MatchDistance = len(text) + 1000

	head := q
	if len(head) > anchorLen {
		head = head[:anchorLen]
	}
	start := dmp.MatchMain(text, head, 0)
	if start < 0 {
		return 0, 0, false
	}
	end := start + len(head)
	if len(q) > anchorLen {
		tail := q[len(q)-anchorLen:]
		dmp.MatchDistance = 1000
		expected := start + len(q) - anchorLen
		t := dmp.MatchMain(text, tail, expected)
		if t < 0 {
			return 0, 0, false
		}
		end = t + anchorLen
	}
	if end > len(text) {
		end = len(text)
	}
	// Anchors can land a character or two inside a word when the quote has
	// typos; widen the span to whole words.
	for start > 0 && isWordByte(text[start-1]) && isWordByte(text[start]) {
		start--
	}
	for end < len(text) && isWordByte(text[end-1]) && isWordByte(text[end]) {
		end++
	}
	span := end - start
	if span <= 0 {
		return 0, 0, false
	}
	lo, hi := float64(len(q))*0.7, float64(len(q))*1.3
	if float64(span) < lo || float64(span) > hi {
		return 0, 0, false
	}
	diffs := dmp.DiffMain(text[start:end], q, false)
	if float64(dmp.DiffLevenshtein(diffs)) > 0.3*float64(len(q)) {
		return 0, 0, false
	}
	return start, end, true
}

// ClosestHeadings returns up to n headings whose sections share the most
// words with the quote.
func ClosestHeadings(src []byte, quote string, n int) []string {
	secs := Sections(src)
	qt := tokens(NormalizeQuote(quote))
	type scored struct {
		name  string
		score float64
	}
	var all []scored
	for i := range secs {
		body := tokens(Normalize(string(src[secs[i].Start:secs[i].BodyEnd])).Text)
		if len(body) == 0 {
			continue
		}
		hit := 0
		for t := range qt {
			if body[t] {
				hit++
			}
		}
		all = append(all, scored{secs[i].Text, float64(hit) / float64(len(qt)+1)})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].score > all[j].score })
	var out []string
	for i := 0; i < len(all) && len(out) < n; i++ {
		out = append(out, all[i].name)
	}
	return out
}

func tokens(s string) map[string]bool {
	out := map[string]bool{}
	for _, f := range strings.FieldsFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127)
	}) {
		if len(f) > 2 {
			out[f] = true
		}
	}
	return out
}

func isWordByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c >= 0x80
}
