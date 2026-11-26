// Package glob matches slash-separated paths against patterns with `*`
// (any run of characters within one path segment), `?` (one character
// within a segment) and `**` (any run of segments, including none).
package glob

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Match reports whether path matches pattern. Both are cleaned and compared
// with forward slashes.
func Match(pattern, path string) bool {
	re, err := compile(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(filepath.ToSlash(filepath.Clean(path)))
}

// MatchAny reports whether path matches any of the patterns.
func MatchAny(patterns []string, path string) bool {
	for _, p := range patterns {
		if Match(p, path) {
			return true
		}
	}
	return false
}

func compile(pattern string) (*regexp.Regexp, error) {
	p := filepath.ToSlash(filepath.Clean(pattern))
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(p); i++ {
		switch c := p[i]; c {
		case '*':
			if i+1 < len(p) && p[i+1] == '*' {
				i++
				if i+1 < len(p) && p[i+1] == '/' {
					// "**/" matches zero or more whole segments.
					i++
					b.WriteString("(?:.*/)?")
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}
