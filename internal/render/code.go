package render

import (
	"bytes"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

const maxCodeBytes = 2 << 20

// Code renders a source file with line numbers, line anchors (L41) and the
// range start-end highlighted. A zero range highlights nothing.
func Code(path string, src []byte, start, end int) (string, error) {
	if len(src) > maxCodeBytes {
		src = append(src[:maxCodeBytes:maxCodeBytes], []byte("\n… file truncated …\n")...)
	}
	lexer := lexers.Match(filepath.Base(path))
	if lexer == nil {
		lexer = lexers.Analyse(string(src))
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)
	opts := []chromahtml.Option{
		chromahtml.WithClasses(true),
		chromahtml.WithLineNumbers(true),
		chromahtml.WithLinkableLineNumbers(true, "L"),
		chromahtml.TabWidth(4),
	}
	if start > 0 && end >= start {
		opts = append(opts, chromahtml.HighlightLines([][2]int{{start, end}}))
	}
	it, err := lexer.Tokenise(nil, string(src))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := chromahtml.New(opts...).Format(&buf, styles.Get("github"), it); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// CodeCSS returns the chroma classes for a light and a dark theme. The dark
// theme applies under prefers-color-scheme: dark and under
// :root[data-theme="dark"].
func CodeCSS() string {
	var light, dark bytes.Buffer
	f := chromahtml.New(chromahtml.WithClasses(true), chromahtml.WithLineNumbers(true))
	_ = f.WriteCSS(&light, styles.Get("github"))
	_ = f.WriteCSS(&dark, styles.Get("github-dark"))
	var b strings.Builder
	b.WriteString("/* light */\n")
	b.WriteString(light.String())
	d := dark.String() + inheritMissingColors(light.String(), dark.String())
	b.WriteString("\n@media (prefers-color-scheme: dark) {\n")
	b.WriteString(prefixRules(d, `:root:not([data-theme="light"]) `))
	b.WriteString("}\n")
	b.WriteString(prefixRules(d, `:root[data-theme="dark"] `))
	return b.String()
}

// inheritMissingColors returns a "color: inherit" rule for every selector
// that sets a text color in the light sheet but not in the dark one. Without
// it, the light color (github's #1f2328 for names and punctuation) stays in
// force on the dark background.
func inheritMissingColors(light, dark string) string {
	colored := func(css string) []string {
		var sels []string
		for _, line := range strings.Split(css, "\n") {
			i := strings.Index(line, "{")
			if i < 0 || !strings.Contains(strings.ReplaceAll(line[i:], "background-color:", ""), "color:") {
				continue
			}
			sel := line[:i]
			if j := strings.Index(sel, "*/"); j >= 0 {
				sel = sel[j+2:]
			}
			sels = append(sels, strings.TrimSpace(sel))
		}
		return sels
	}
	inDark := map[string]bool{}
	for _, sel := range colored(dark) {
		inDark[sel] = true
	}
	var b strings.Builder
	for _, sel := range colored(light) {
		if !inDark[sel] {
			inDark[sel] = true
			b.WriteString(sel + " { color: inherit }\n")
		}
	}
	return b.String()
}

// prefixRules prefixes every selector in a flat CSS sheet.
func prefixRules(css, prefix string) string {
	var b strings.Builder
	for _, line := range strings.Split(css, "\n") {
		i := strings.Index(line, "{")
		if i < 0 {
			b.WriteString(line + "\n")
			continue
		}
		// Keep a leading comment like "/* Background */ .bg { ... }".
		sel := line[:i]
		comment := ""
		if j := strings.Index(sel, "*/"); j >= 0 {
			comment, sel = sel[:j+2]+" ", sel[j+2:]
		}
		parts := strings.Split(sel, ",")
		for k, p := range parts {
			parts[k] = prefix + strings.TrimSpace(p)
		}
		b.WriteString(comment + strings.Join(parts, ", ") + " " + line[i:] + "\n")
	}
	return b.String()
}

// LineCount counts the lines of a source file.
func LineCount(src []byte) int {
	n := bytes.Count(src, []byte("\n"))
	if len(src) > 0 && src[len(src)-1] != '\n' {
		n++
	}
	return n
}
