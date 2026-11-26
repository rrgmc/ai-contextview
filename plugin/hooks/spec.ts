// Pure helpers: headings, quote matching, sections and line ranges. The Go
// bridge does this properly (internal/specsrc, internal/resolve); this is the
// small subset the pane needs when it runs without the bridge.

export type Heading = { title: string; level: number; offset: number }

const HEADING = /^(#{1,6})[ \t]+(.+?)[ \t#]*$/gm

export function headings(md: string): Heading[] {
  const out: Heading[] = []
  for (const m of md.matchAll(HEADING)) {
    // Inline code marks are dropped, as the bridge's section names drop them.
    out.push({ title: (m[2] ?? '').replace(/`/g, ''), level: (m[1] ?? '#').length, offset: m.index ?? 0 })
  }
  return out
}

export type QuoteMatch = { start: number; end: number; how: 'exact' | 'normalized' }

/** Finds `quote` in `md`, exactly or with runs of whitespace collapsed. */
export function findQuote(md: string, quote: string): QuoteMatch | null {
  const q = quote.trim()
  if (q === '') return null
  const exact = md.indexOf(q)
  if (exact >= 0) return { start: exact, end: exact + q.length, how: 'exact' }

  // Collapse whitespace in both, keeping a map back to offsets in md.
  const map: number[] = []
  let flat = ''
  let inSpace = false
  for (let i = 0; i < md.length; i++) {
    const c = md[i] ?? ''
    if (/\s/.test(c)) {
      if (!inSpace) {
        flat += ' '
        map.push(i)
      }
      inSpace = true
    } else {
      flat += c
      map.push(i)
      inSpace = false
    }
  }
  const nq = q.replace(/\s+/g, ' ')
  const at = flat.indexOf(nq)
  if (at < 0) return null
  return { start: map[at] ?? 0, end: (map[at + nq.length - 1] ?? 0) + 1, how: 'normalized' }
}

/** The section around `offset`: its heading up to the next heading of the same or a higher level. */
export function sectionAt(md: string, offset: number): { title: string; start: number; end: number } {
  const hs = headings(md)
  let cur = -1
  hs.forEach((one, i) => {
    if (one.offset <= offset) cur = i
  })
  const h = hs[cur]
  if (!h) {
    return { title: '', start: 0, end: hs[0]?.offset ?? md.length }
  }
  let end = md.length
  for (const next of hs.slice(cur + 1)) {
    if (next.level <= h.level) {
      end = next.offset
      break
    }
  }
  return { title: h.title, start: h.offset, end }
}

/** The section whose heading matches `title`, ignoring case and leading #s. */
export function sectionByTitle(md: string, title: string): { title: string; start: number; end: number } | null {
  const want = title.replace(/^#+\s*/, '').trim().toLowerCase()
  const h = headings(md).find(one => one.title.trim().toLowerCase() === want)
  return h ? sectionAt(md, h.offset) : null
}

/** Bolds the quote in a section when it sits on one line with no markup of its own. */
export function emphasize(md: string, start: number, end: number): string {
  const q = md.slice(start, end)
  if (/[\n*_`[\]]/.test(q)) return md
  return md.slice(0, start) + '**' + q + '**' + md.slice(end)
}

/** Parses `N` or `N-M` into a 1-based inclusive range. */
export function parseLines(lines: string): { start: number; end: number } | null {
  const m = /^\s*(\d+)\s*(?:-\s*(\d+))?\s*$/.exec(lines)
  if (!m) return null
  const start = Number(m[1] ?? 0)
  const end = m[2] ? Number(m[2]) : start
  return start >= 1 && end >= start ? { start, end } : null
}

/**
 * Finds a Go declaration by its last name segment, e.g. `pkg.(*T).Method`
 * looks for `func (... *T) Method(`. A crude stand-in for go/packages.
 */
export function findGoSymbol(src: string, symbol: string): { start: number; end: number } | null {
  const parts = symbol.replace(/[()*]/g, '').split('.').filter(Boolean)
  const name = parts[parts.length - 1] ?? ''
  const recv = parts.length >= 3 ? (parts[parts.length - 2] ?? '') : ''
  const lines = src.split('\n')
  const esc = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const pats = recv
    ? [new RegExp(`^func\\s*\\([^)]*\\b${esc(recv)}\\)\\s*${esc(name)}\\b`)]
    : [
        new RegExp(`^func\\s+${esc(name)}\\b`),
        new RegExp(`^type\\s+${esc(name)}\\b`),
        new RegExp(`^(var|const)\\s+${esc(name)}\\b`),
      ]
  const at = lines.findIndex(l => pats.some(p => p.test(l)))
  if (at < 0) return null
  // Take the doc comment above the declaration too, as go/packages does.
  let first = at
  while (first > 0 && /^\s*\/\//.test(lines[first - 1] ?? '')) first--
  // A declaration that opens a block ends at the first "}" or ")" at column 0.
  let end = at
  if (/[{(]\s*$/.test(lines[at] ?? '')) {
    for (let i = at + 1; i < lines.length; i++) {
      if (/^[})]/.test(lines[i] ?? '')) {
        end = i
        break
      }
    }
  }
  return { start: first + 1, end: end + 1 }
}

/** Cuts 1-based lines start..end out of src, at most `max` of them. */
export function sliceLines(src: string, start: number, end: number, max = 300): string {
  const lines = src.split('\n')
  return lines.slice(start - 1, Math.min(end, start - 1 + max)).join('\n')
}

/**
 * Splits md into the blocks before, around and after start..end. Cuts fall
 * only at a line that follows a blank line outside a fenced code block, so
 * each part is whole markdown. The quote is bolded in the middle part.
 */
export function splitAround(md: string, start: number, end: number): { before: string; focus: string; after: string } {
  const cuts: number[] = [0]
  let inFence = false
  let prevBlank = false
  let at = 0
  for (const line of md.split('\n')) {
    if (at > 0 && prevBlank && !inFence) cuts.push(at)
    if (/^\s*(```|~~~)/.test(line)) inFence = !inFence
    prevBlank = line.trim() === ''
    at += line.length + 1
  }
  let lo = 0
  for (const c of cuts) if (c <= start) lo = c
  const hi = cuts.find(c => c >= end && c > lo) ?? md.length
  return {
    before: md.slice(0, lo),
    focus: emphasize(md.slice(lo, hi), start - lo, end - lo),
    after: md.slice(hi),
  }
}

/**
 * Whether an option label already means "decide later", such as "Decide later",
 * "Decide this later" or "Defer". The question form then leaves out its own
 * "Decide later". Keep it in step with isLaterLabel in internal/daemon/views.go.
 */
export function isLaterLabel(label: string): boolean {
  const l = label
    .toLowerCase()
    .split(/[^\p{L}\p{N}]+/u)
    .filter(w => w !== '')
    .join(' ')
  for (const p of ['decide later', 'decide it later', 'decide this later', 'decide that later']) {
    if (l === p || l.startsWith(p + ' ')) return true
  }
  return ['later', 'defer', 'defer it', 'defer this', 'defer for now', 'defer decision', 'defer the decision'].includes(l)
}

/** The number of lines of text; a final line without a newline counts. */
export function lineCount(text: string): number {
  if (text === '') return 0
  const n = text.split('\n').length
  return text.endsWith('\n') ? n - 1 : n
}

/** The offsets of lines start..end (1-based, inclusive) of text, the last newline excluded. */
export function lineOffsets(text: string, start: number, end: number): { start: number; end: number } {
  let s = -1
  let line = 1
  for (let i = 0; i <= text.length; i++) {
    if (line === start && s < 0) s = i
    if (i === text.length) break
    if (text[i] === '\n') {
      if (line === end) return { start: s < 0 ? text.length : s, end: i }
      line++
    }
  }
  return { start: s < 0 ? text.length : s, end: text.length }
}

/** Turns a glob (`*`, `?`, `**`) into a RegExp, as internal/glob. */
export function globRegExp(pattern: string): RegExp {
  let re = '^'
  for (let i = 0; i < pattern.length; i++) {
    const c = pattern[i]
    if (c === '*') {
      if (pattern[i + 1] === '*') {
        i++
        if (pattern[i + 1] === '/') {
          i++
          re += '(?:.*/)?'
        } else re += '.*'
      } else re += '[^/]*'
    } else if (c === '?') re += '[^/]'
    else re += (c ?? '').replace(/[.+^${}()|[\]\\]/g, '\\$&')
  }
  return new RegExp(re + '$')
}

export const matchAny = (patterns: string[], path: string) => patterns.some(p => globRegExp(p).test(path))

/**
 * Whether a written file can become the working document, as internal/hook
 * Counts: markdown that is not bookkeeping. Non-empty patterns replace the rules.
 */
export function countsAsDoc(path: string, home: string, patterns: string[], cwd = ''): boolean {
  if (!/\.(md|markdown)$/i.test(path)) return false
  if (patterns.length > 0) return matchAny(patterns, path)
  const base = (path.split('/').pop() ?? '').toLowerCase()
  if (['claude.md', 'claude.local.md', 'agents.md', 'changelog.md'].includes(base)) return false
  const claude = home ? `${home.replace(/\/$/, '')}/.claude/` : ''
  if (claude && path.startsWith(claude)) {
    const rest = path.slice(claude.length)
    return rest.startsWith('specs/') || rest.startsWith('plans/')
  }
  // Inside the session's folder, only the folders below it count, so a
  // desktop-app worktree (<repo>/.claude/worktrees/x) is not a dot-folder.
  const base_ = cwd ? cwd.replace(/\/$/, '') + '/' : ''
  const inside = base_ !== '' && path.startsWith(base_)
  const parts = (inside ? path.slice(base_.length) : path).split('/').slice(0, -1)
  for (let i = 0; i < parts.length; i++) {
    const part = parts[i] ?? ''
    if (part === 'changelog.d') return false
    if (part.startsWith('.') && part !== '.' && part !== '..') {
      if (part === '.claude' && parts[i + 1] === 'specs') continue
      return false
    }
  }
  return true
}

const SPEC_FOLDERS = new Set(['specs', 'spec', 'rfc', 'rfcs', 'design', 'designs', 'adr', 'adrs'])
const SPEC_HEADING = /^#{1,6}\s+(requirements|goals?|non-goals|design|acceptance criteria|open questions|motivation|proposal|decisions?)\b/gim

/** How much a written file looks like a spec, as internal/daemon workDoc.score. */
export function specScore(path: string, text: string, home: string, specSkill: boolean, edits: number): number {
  let n = specSkill ? 2 : 0
  const claude = home ? `${home.replace(/\/$/, '')}/.claude/` : ''
  const inClaude = !!claude && (path.startsWith(claude + 'specs/') || path.startsWith(claude + 'plans/'))
  if (inClaude || path.split('/').slice(0, -1).some(d => SPEC_FOLDERS.has(d.toLowerCase()))) n++
  if (/^kind:\s*spec\s*$/m.test(text) || (text.match(SPEC_HEADING) ?? []).length >= 2) n++
  if (edits >= 2) n++
  return n
}
