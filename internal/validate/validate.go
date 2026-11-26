// Package validate checks MCP tool inputs against the rules in
// docs/01-protocol.md. All problems are collected into one error so Claude can
// fix them in one retry.
package validate

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/rrgmc/ai-specview/internal/protocol"
)

// Limits from the protocol.
const (
	MaxQuestions = 6
	MinOptions   = 2
	MaxOptions   = 6
	MaxDocRefs   = 4
	MaxCodeRefs  = 4
	MinQuote     = 3
	MaxQuote     = 300
)

// Errors collects validation problems.
type Errors []string

func (e *Errors) add(format string, args ...any) { *e = append(*e, fmt.Sprintf(format, args...)) }

// Err returns nil when there are no problems.
func (e Errors) Err() error {
	if len(e) == 0 {
		return nil
	}
	return errors.New("invalid input:\n- " + strings.Join(e, "\n- "))
}

// Ask validates the ask tool input. refsOptional drops the rule that each
// question needs a ref; it is set in ask mode, where a question may be about
// neither a spec nor code.
func Ask(in *protocol.AskInput, refsOptional bool) error {
	var errs Errors
	if n := len(in.Questions); n < 1 || n > MaxQuestions {
		errs.add("questions: got %d, want 1-%d", n, MaxQuestions)
	}
	ids := map[string]bool{}
	for i, q := range in.Questions {
		label := fmt.Sprintf("question %d", i+1)
		if q.ID != "" {
			label = fmt.Sprintf("question %q", q.ID)
		}
		if q.ID == "" {
			errs.add("%s: missing id", label)
		} else if ids[q.ID] {
			errs.add("%s: duplicate id", label)
		}
		ids[q.ID] = true
		if strings.TrimSpace(q.Text) == "" {
			errs.add("%s: missing text", label)
		}
		if n := len(q.Options); n < MinOptions || n > MaxOptions {
			errs.add("%s: got %d options, want %d-%d", label, n, MinOptions, MaxOptions)
		}
		labels := map[string]bool{}
		recommended := 0
		for j, o := range q.Options {
			if o.Recommended {
				recommended++
			}
			if strings.TrimSpace(o.Label) == "" {
				errs.add("%s: option %d has no label", label, j+1)
			} else if labels[o.Label] {
				errs.add("%s: duplicate option label %q", label, o.Label)
			}
			labels[o.Label] = true
		}
		if recommended > 1 {
			errs.add("%s: %d options are marked recommended, at most 1", label, recommended)
		}
		if !refsOptional && q.SpecQuote == "" && len(q.DocRefs) == 0 && len(q.CodeRefs) == 0 {
			errs.add("%s: needs at least one of spec_quote, doc_refs or code_refs", label)
		}
		refs(&errs, label, q.SpecQuote, q.SpecSection, q.DocRefs, q.CodeRefs)
	}
	return errs.Err()
}

// Focus validates the focus tool input.
func Focus(in *protocol.FocusInput) error {
	var errs Errors
	if in.SpecQuote == "" && len(in.DocRefs) == 0 && len(in.CodeRefs) == 0 {
		errs.add("needs at least one of spec_quote, doc_refs or code_refs")
	}
	refs(&errs, "focus", in.SpecQuote, in.SpecSection, in.DocRefs, in.CodeRefs)
	return errs.Err()
}

func refs(errs *Errors, label, specQuote, specSection string, docs []protocol.DocRef, code []protocol.CodeRef) {
	if specQuote == "" && specSection != "" {
		errs.add("%s: spec_section needs spec_quote", label)
	}
	nDocs := len(docs)
	if specQuote != "" {
		nDocs++
		quote(errs, label+" spec_quote", specQuote)
	}
	if nDocs > MaxDocRefs {
		errs.add("%s: %d doc refs, at most %d", label, nDocs, MaxDocRefs)
	}
	for i, d := range docs {
		quote(errs, fmt.Sprintf("%s doc_refs[%d]", label, i), d.Quote)
	}
	if len(code) > MaxCodeRefs {
		errs.add("%s: %d code refs, at most %d", label, len(code), MaxCodeRefs)
	}
	for i, c := range code {
		l := fmt.Sprintf("%s code_refs[%d]", label, i)
		if c.Path == "" {
			errs.add("%s: missing path", l)
		}
		if (c.Symbol == "") == (c.Lines == "") {
			errs.add("%s: give exactly one of symbol or lines", l)
		}
	}
}

func quote(errs *Errors, label, q string) {
	n := utf8.RuneCountInString(strings.TrimSpace(q))
	if n < MinQuote || n > MaxQuote {
		errs.add("%s: quote is %d characters, want %d-%d", label, n, MinQuote, MaxQuote)
	}
}
