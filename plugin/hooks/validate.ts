// The input rules of internal/validate, for the tools the pane serves itself.
// Each returns the list of problems; an empty list means the input is valid.

export const MAX_QUESTIONS = 6
export const MIN_OPTIONS = 2
export const MAX_OPTIONS = 6
export const MAX_DOC_REFS = 4
export const MAX_CODE_REFS = 4
export const MIN_QUOTE = 3
export const MAX_QUOTE = 300
export const MAX_SUMMARY = 8000

export type OptionArg = { label?: string; description?: string; recommended?: boolean }
export type DocRefArg = { path?: string; quote?: string; section?: string }
export type CodeRefArg = { path?: string; symbol?: string; lines?: string }
export type QuestionArg = {
  id?: string
  text?: string
  options?: OptionArg[]
  multi_select?: boolean
  allow_other?: boolean
  spec_quote?: string
  spec_section?: string
  doc_refs?: DocRefArg[]
  code_refs?: CodeRefArg[]
}

const chars = (s: string) => [...s.trim()].length

function quote(errs: string[], label: string, q: string) {
  const n = chars(q)
  if (n < MIN_QUOTE || n > MAX_QUOTE) errs.push(`${label}: quote is ${n} characters, want ${MIN_QUOTE}-${MAX_QUOTE}`)
}

function refs(errs: string[], label: string, specQuote: string, specSection: string, docs: DocRefArg[], code: CodeRefArg[]) {
  if (specQuote === '' && specSection !== '') errs.push(`${label}: spec_section needs spec_quote`)
  let nDocs = docs.length
  if (specQuote !== '') {
    nDocs++
    quote(errs, `${label} spec_quote`, specQuote)
  }
  if (nDocs > MAX_DOC_REFS) errs.push(`${label}: ${nDocs} doc refs, at most ${MAX_DOC_REFS}`)
  docs.forEach((d, i) => quote(errs, `${label} doc_refs[${i}]`, d.quote ?? ''))
  if (code.length > MAX_CODE_REFS) errs.push(`${label}: ${code.length} code refs, at most ${MAX_CODE_REFS}`)
  code.forEach((c, i) => {
    const l = `${label} code_refs[${i}]`
    if (!c.path) errs.push(`${l}: missing path`)
    if (!c.symbol === !c.lines) errs.push(`${l}: give exactly one of symbol or lines`)
  })
}

export function focus(args: Record<string, unknown>): string[] {
  const errs: string[] = []
  const specQuote = String(args.spec_quote ?? '')
  const docs = (args.doc_refs as DocRefArg[] | undefined) ?? []
  const code = (args.code_refs as CodeRefArg[] | undefined) ?? []
  if (specQuote === '' && docs.length === 0 && code.length === 0) {
    errs.push('needs at least one of spec_quote, doc_refs or code_refs')
  }
  refs(errs, 'focus', specQuote, String(args.spec_section ?? ''), docs, code)
  return errs
}

export function ask(args: Record<string, unknown>): string[] {
  const errs: string[] = []
  const qs = (args.questions as QuestionArg[] | undefined) ?? []
  if (qs.length < 1 || qs.length > MAX_QUESTIONS) errs.push(`questions: got ${qs.length}, want 1-${MAX_QUESTIONS}`)
  const ids = new Set<string>()
  qs.forEach((q, i) => {
    const label = q.id ? `question "${q.id}"` : `question ${i + 1}`
    if (!q.id) errs.push(`${label}: missing id`)
    else if (ids.has(q.id)) errs.push(`${label}: duplicate id`)
    if (q.id) ids.add(q.id)
    if (!q.text?.trim()) errs.push(`${label}: missing text`)
    const opts = q.options ?? []
    if (opts.length < MIN_OPTIONS || opts.length > MAX_OPTIONS) {
      errs.push(`${label}: got ${opts.length} options, want ${MIN_OPTIONS}-${MAX_OPTIONS}`)
    }
    const labels = new Set<string>()
    let recommended = 0
    opts.forEach((o, j) => {
      if (o.recommended) recommended++
      const l = o.label ?? ''
      if (!l.trim()) errs.push(`${label}: option ${j + 1} has no label`)
      else if (labels.has(l)) errs.push(`${label}: duplicate option label "${l}"`)
      labels.add(l)
    })
    if (recommended > 1) errs.push(`${label}: ${recommended} options are marked recommended, at most 1`)
    const specQuote = q.spec_quote ?? ''
    const docs = q.doc_refs ?? []
    const code = q.code_refs ?? []
    if (specQuote === '' && docs.length === 0 && code.length === 0) {
      errs.push(`${label}: needs at least one of spec_quote, doc_refs or code_refs`)
    }
    refs(errs, label, specQuote, q.spec_section ?? '', docs, code)
  })
  return errs
}

/** Turns a "(Recommended)" label suffix into the recommended flag, as the bridge does. */
export function normalizeRecommended(o: OptionArg): { label: string; description: string; recommended: boolean } {
  let label = (o.label ?? '').trim()
  let recommended = !!o.recommended
  const suffix = '(recommended)'
  if (label.toLowerCase().endsWith(suffix)) {
    label = label.slice(0, -suffix.length).trim()
    recommended = true
  }
  return { label, description: o.description ?? '', recommended }
}

/** The bytes of s in UTF-8, as Go's len() counts them. */
export function utf8Length(s: string): number {
  let n = 0
  for (const ch of s) {
    const c = ch.codePointAt(0) ?? 0
    n += c < 0x80 ? 1 : c < 0x800 ? 2 : c < 0x10000 ? 3 : 4
  }
  return n
}
