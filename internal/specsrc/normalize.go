// Package specsrc works on markdown source: normalization, quote matching,
// section indexes and artifact roots. Both the bridge (quote pre-check) and the
// daemon (highlighting) use it, so a passing pre-check means the highlight
// lands.
package specsrc

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Normalized is a normalized view of a source text. Text[i] came from the
// source bytes [Start[i], End[i]).
type Normalized struct {
	Text  string
	Start []int
	End   []int
}

// SourceRange maps a normalized byte range [i, j) back to source bytes.
func (n *Normalized) SourceRange(i, j int) (int, int) {
	if j <= i || i < 0 || j > len(n.Text) {
		return 0, 0
	}
	return n.Start[i], n.End[j-1]
}

var runeMap = map[rune]string{
	'‘': "'", '’': "'", '‚': "'", '‛': "'", '′': "'",
	'“': `"`, '”': `"`, '„': `"`, '‟': `"`, '″': `"`,
	'‐': "-", '‑': "-", '‒': "-", '–': "-", '—': "-", '―': "-", '−': "-",
	'…': "...",
	' ': " ", ' ': " ", ' ': " ",
}

type normalizer struct {
	src   string
	out   strings.Builder
	start []int
	end   []int
	space bool // last emitted byte was a space (or nothing emitted yet)
}

func (n *normalizer) emit(s string, from, to int) {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' {
			if n.space {
				continue
			}
			n.space = true
		} else {
			n.space = false
		}
		n.out.WriteByte(c)
		n.start = append(n.start, from)
		n.end = append(n.end, to)
	}
}

// Normalize returns the normalized form of a markdown text with its index map:
// NFC, lower case, inline markdown syntax and HTML tags removed, block markers
// (headings, list bullets, quotes, table pipes) removed, typographic quotes and
// dashes mapped to ASCII, and all whitespace collapsed to one space.
func Normalize(src string) *Normalized {
	n := &normalizer{src: src, space: true}
	lineStart := true
	i := 0
	for i < len(src) {
		if lineStart {
			i = n.skipBlockMarkers(i)
			lineStart = false
			if i >= len(src) {
				break
			}
		}
		r, size := utf8.DecodeRuneInString(src[i:])
		switch {
		case r == '\n':
			n.emit(" ", i, i+1)
			lineStart = true
			i++
			continue
		case unicode.IsSpace(r):
			n.emit(" ", i, i+size)
		case r == '*' || r == '_' || r == '`' || r == '~':
			// inline emphasis / code / strikethrough markers
		case r == '|':
			n.emit(" ", i, i+size)
		case r == '!' && i+1 < len(src) && src[i+1] == '[':
			// image marker; the [alt] is handled next
		case r == '[':
			// link text start; the closing ](url) is skipped below
		case r == ']':
			if i+1 < len(src) && src[i+1] == '(' {
				if j := strings.IndexByte(src[i+1:], ')'); j >= 0 {
					i += 1 + j + 1
					continue
				}
			}
		case r == '<' && i+1 < len(src) && (isASCIILetter(src[i+1]) || src[i+1] == '/'):
			if j := strings.IndexByte(src[i:], '>'); j >= 0 && !strings.ContainsRune(src[i:i+j], '\n') {
				i += j + 1
				continue
			}
			n.emit("<", i, i+size)
		case r == '\\' && i+1 < len(src) && isASCIIPunct(src[i+1]):
			// Escaped punctuation: drop the backslash. Characters the
			// normalizer strips anyway are dropped too, so `\_` and `_` agree.
			if c := src[i+1]; strings.IndexByte("*_`~[]|", c) < 0 {
				n.emit(string(c), i, i+2)
			}
			i += 2
			continue
		default:
			if m, ok := runeMap[r]; ok {
				n.emit(m, i, i+size)
			} else {
				s := strings.ToLower(norm.NFC.String(string(r)))
				n.emit(s, i, i+size)
			}
		}
		i += size
	}
	text := n.out.String()
	// Trim a trailing space.
	if strings.HasSuffix(text, " ") {
		text = text[:len(text)-1]
		n.start = n.start[:len(text)]
		n.end = n.end[:len(text)]
	}
	return &Normalized{Text: text, Start: n.start, End: n.end}
}

// skipBlockMarkers skips leading block syntax on a line: indentation, heading
// hashes, block quote markers, list bullets and ordered list numbers.
func (n *normalizer) skipBlockMarkers(i int) int {
	src := n.src
	for {
		j := i
		for j < len(src) && (src[j] == ' ' || src[j] == '\t') {
			j++
		}
		switch {
		case j < len(src) && src[j] == '>':
			i = j + 1
			continue
		case j < len(src) && src[j] == '#':
			k := j
			for k < len(src) && src[k] == '#' {
				k++
			}
			if k-j <= 6 && (k == len(src) || src[k] == ' ' || src[k] == '\n') {
				return k
			}
			return j
		case j+1 < len(src) && (src[j] == '-' || src[j] == '+' || src[j] == '*') && (src[j+1] == ' ' || src[j+1] == '\t'):
			i = j + 2
			// task list box
			if strings.HasPrefix(src[i:], "[ ] ") || strings.HasPrefix(src[i:], "[x] ") || strings.HasPrefix(src[i:], "[X] ") {
				i += 4
			}
			continue
		default:
			k := j
			for k < len(src) && src[k] >= '0' && src[k] <= '9' {
				k++
			}
			if k > j && k-j <= 9 && k+1 < len(src) && (src[k] == '.' || src[k] == ')') && src[k+1] == ' ' {
				i = k + 2
				continue
			}
			return j
		}
	}
}

func isASCIILetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

func isASCIIPunct(c byte) bool {
	return strings.IndexByte("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", c) >= 0
}

// NormalizeQuote normalizes a quote the same way as the source, without the
// index map.
func NormalizeQuote(q string) string {
	return Normalize(norm.NFC.String(q)).Text
}
