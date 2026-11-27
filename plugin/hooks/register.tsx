// The pane mode of ai-contextview: a Claude Code mod that draws the spec, the
// code and the questions in a pane beside the chat.
//
// It registers its own tools (spec_show, focus, ask, ask_wait, ask_mode, spec_done),
// listed as mcp__<plugin>__<name>, beside the bridge's MCP tools of the same
// names. The spec-elaborate-mod skill uses these; spec-elaborate uses the
// bridge and the browser page. No daemon runs for the pane. While a spec is
// open in the pane, or ask mode is on, the built-in AskUserQuestion is blocked.

import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register } from 'claude-code'

import type {
  Answer,
  AskOutput,
  AskView,
  CodeView,
  DocRefView,
  Draft,
  PaneUi,
  QuestionView,
  QuoteView,
  SpecView,
  WorkDoc,
} from '../types'
import * as spec from './spec'
import { TOOLS } from './tools'
import * as check from './validate'

const PANE = 'ai-contextview'
const TOOL_NAMES = new Set(TOOLS.map(t => t.name))

// How long ask and ask_wait wait before answering pending, as the bridge's
// AI_CONTEXTVIEW_ASK_WAIT default. A tool.call hook may run 20 minutes.
const ASK_WAIT_MS = 10 * 60_000

// A hook's own time is capped at 10 seconds and a $.clock wait counts toward
// it, but time inside any other $ call does not (up to 20 minutes on
// tool.call). So the wait pauses between checks with `sleep 1` run through
// $.process.run.
const PAUSE = ['/bin/sleep', '1'] as const

// Change a tag when the stored shape changes: it declines an older value.
const viewAtom = atom({ plugin: 'ai-contextview', key: 'view' } as const, null as SpecView | null, {
  shape: 'view-4',
})
const uiAtom = atom({ plugin: 'ai-contextview', key: 'ui' } as const, { file: '', shown: -1, codeTab: 0 } as PaneUi, {
  shape: 'ui-2',
})
const askAtom = atom({ plugin: 'ai-contextview', key: 'ask' } as const, null as AskView | null, {
  shape: 'ask-3',
})
// Ask mode: every question of the session goes through the pane, with or without a spec.
const modeAtom = atom({ plugin: 'ai-contextview', key: 'mode' } as const, false, { shape: 'mode-1' })
// Set when ask mode is turned off: the session asks in the chat again, even
// with a spec open. Turning ask mode on clears it.
const releasedAtom = atom({ plugin: 'ai-contextview', key: 'released' } as const, false, { shape: 'released-1' })
// As protocol.OffNext.
const OFF_NEXT =
  'ai-contextview is off for this session. Ask every question in the chat or with the built-in question tool from now on, not with the ask tool, even while a spec is open. The spec stays in the viewer.'
const noWork = (): WorkDoc => ({
  explicit: false,
  spec: false,
  paths: [],
  specSkill: false,
  doc: '',
  autoShown: '',
  provisional: false,
  edits: {},
})
const workAtom = atom({ plugin: 'ai-contextview', key: 'work' } as const, noWork(), { shape: 'work-3' })
// The score a written file needs in spec mode without paths, as the daemon's.
const SCORE_THRESHOLD = 2
const SPEC_SKILL = /spec|rfc|design/i
const WRITE_TOOLS = new Set(['Write', 'Edit', 'MultiEdit'])

// Messages returned to Claude, as internal/tools sends them.
const MSG_IN_CHAT = 'The user will answer in the chat. Wait for their message.'
const MSG_PENDING = 'Still waiting for the user. Call ask_wait with this id to keep waiting.'
const MSG_DISMISSED =
  'The user skipped these questions. Continue without these decisions; ask in chat only if one is essential.'
const MSG_DEFERRED =
  "The user chose to decide some questions later (see deferred). Add each to the spec's open questions with its refs, " +
  'continue without the decision, and ask them again before finishing.'
// As internal/hook's Reason.
const BLOCK_REASON =
  'A spec is open in ai-contextview for this session. Ask this with the ai-contextview `ask` tool ' +
  'instead of AskUserQuestion: quote the spec verbatim with spec_quote and ' +
  'spec_section, and point at the code with code_refs.' +
  " To ask for approval of an action that needs the user's permission, ask in plain chat instead."
// As internal/hook's ReasonAskMode.
const BLOCK_REASON_ASK_MODE =
  'Ask mode is on in ai-contextview for this session. Ask this with the ai-contextview `ask` tool ' +
  'instead of AskUserQuestion. Refs are optional: add code_refs or doc_refs when the question is about them.' +
  " To ask for approval of an action that needs the user's permission, ask in plain chat instead."

type Args = Record<string, unknown>

const MAX_MARKDOWN = 50_000
const MAX_CODE = 40_000
// The pane splits into two columns from this many cells across.
const WIDE = 100
// codeTab while questions wait: the side column shows the form, not a code ref.
const FORM_TAB = -1

type Ctx = {
  cwd: string
  // The user's home folder, for paths that start with ~.
  home: string
  // Outcomes the person chose, by question set, for the hook that waits on it.
  results: Map<string, AskOutput>
}

// Module state: it starts over on a reload, and session.start sets cwd and home again.
const ctx: Ctx = { cwd: '', home: '', results: new Map() }

/** A leading ~ or ~/ is the home folder, as in Claude Code's file tools and the bridge. */
const expand = (p: string) =>
  ctx.home && (p === '~' || p.startsWith('~/')) ? ctx.home.replace(/[\\/]$/, '') + p.slice(1) : p
const abs = (p: string) => {
  const q = expand(p)
  return q.startsWith('/') ? q : `${ctx.cwd.replace(/\/$/, '')}/${q}`
}
const rel = (p: string) => (ctx.cwd && p.startsWith(ctx.cwd + '/') ? p.slice(ctx.cwd.length + 1) : p)
const emptyDraft = (): Draft => ({ selected: [], other: '', notes: '', later: false })
const noQuote: QuoteView = { quote: '', quoteStart: -1, quoteEnd: -1, quoteSection: '' }
/** Text on one line, runs of whitespace collapsed, for a caption. */
const oneLine = (s: string) => s.replace(/\s+/g, ' ').trim()

function fail(errs: string[]): never {
  throw new Error('fix these and call again:\n- ' + errs.join('\n- '))
}

/** Finds quote in text; adds an error naming the file when it is missing. */
function locate(text: string, quote: string, section: string, file: string, label: string, errs: string[]): QuoteView & { how: string } {
  if (!quote) return { ...noQuote, quoteSection: section, how: '' }
  const m = spec.findQuote(text, quote)
  if (!m) {
    errs.push(`${label}: quote ${JSON.stringify(quote)} not found in ${rel(file)} (copy it verbatim from the file)`)
    return { ...noQuote, quote, quoteSection: section, how: 'none' }
  }
  const s = spec.sectionAt(text, m.start)
  return { quote, quoteStart: m.start, quoteEnd: m.end, quoteSection: s.title || section, how: m.how }
}

const sectionOf = (text: string, q: QuoteView) => (q.quoteStart >= 0 ? spec.sectionAt(text, q.quoteStart).start : -1)

const MARKDOWN = /\.(md|markdown)$/i
// How deep a folder spec is listed, as internal/specsrc.
const MAX_DEPTH = 3

const join = (dir: string, p: string) => `${dir.replace(/\/$/, '')}/${p}`
const dirOf = (p: string) => p.replace(/\/[^/]*$/, '') || '/'

/** Resolves . and .. in an absolute path. */
function clean(p: string): string {
  const out: string[] = []
  for (const part of p.split('/')) {
    if (part === '' || part === '.') continue
    if (part === '..') out.pop()
    else out.push(part)
  }
  return '/' + out.join('/')
}

/** The path of p from dir, or p itself when it is outside dir. */
const under = (dir: string, p: string) => (p.startsWith(dir.replace(/\/$/, '') + '/') ? p.slice(dir.replace(/\/$/, '').length + 1) : p)

/** Markdown files under dir, up to MAX_DEPTH deep, dot-entries skipped; relative and sorted. */
async function listMarkdown($: EngineInterface, dir: string, prefix = '', depth = 1): Promise<string[]> {
  const out: string[] = []
  for (const f of await $.fs.list(prefix ? join(dir, prefix) : dir)) {
    if (f.name.startsWith('.')) continue
    const relPath = prefix ? `${prefix}/${f.name}` : f.name
    if (f.kind === 'dir') {
      if (depth < MAX_DEPTH) out.push(...(await listMarkdown($, dir, relPath, depth + 1)))
    } else if (MARKDOWN.test(f.name)) {
      out.push(relPath)
    }
  }
  return out.sort()
}

/** The spec's files as the bridge's LoadRoot finds them: a markdown file, or a folder of them. */
async function loadRoot($: EngineInterface, path: string, primary?: string) {
  const p = clean(abs(path))
  const st = await $.fs.stat(p)
  if (st.kind !== 'dir') {
    if (primary) throw new Error('primary is only allowed for folders')
    if (!MARKDOWN.test(p)) throw new Error(`${p} is not a markdown file (.md or .markdown)`)
    return { dir: dirOf(p), isDir: false, primary: p, files: [p] }
  }
  const names = await listMarkdown($, p)
  if (names.length === 0) throw new Error(`${p} contains no markdown files`)
  let pick: string | undefined
  if (primary) {
    primary = expand(primary)
    const want = primary.startsWith('/') ? under(p, clean(primary)) : clean(join(p, primary)).slice(p.length + 1)
    if (!names.includes(want)) throw new Error(`primary "${primary}" is not a markdown file inside ${p}`)
    pick = want
  } else {
    pick = ['proposal.md', 'spec.md', 'README.md'].find(n => names.includes(n)) ?? names[0]
  }
  const first = join(p, pick ?? '')
  return { dir: p, isDir: true, primary: first, files: [first, ...names.map(n => join(p, n)).filter(f => f !== first)] }
}

async function showSpec($: EngineInterface, args: Args) {
  const root = await loadRoot($, String(args.path ?? ''), args.primary as string | undefined)
  if (!root.isDir && Array.isArray(args.changed) && args.changed.length > 0) {
    throw new Error('changed is only allowed when path is a folder; use changed_sections')
  }
  const file = root.primary
  const text: string = await $.fs.read(file)
  // Showing another spec starts a new spec session: an earlier off no longer
  // applies. Showing the released spec again keeps it released.
  if ((await read($, viewAtom))?.specPath !== file) await update($, releasedAtom, () => false)
  const changed = (args.changed as { path?: string }[] | undefined) ?? []
  const view: SpecView = {
    ...noQuote,
    specPath: file,
    title: rel(file),
    dir: root.dir,
    isDir: root.isDir,
    files: root.files,
    changedFiles: changed.filter(c => c.path).map(c => clean(join(root.dir, c.path ?? ''))),
    docs: [],
    text,
    changedSections: (args.changed_sections as string[] | undefined) ?? [],
    match: '',
    code: [],
    error: '',
    isDone: false,
    summary: '',
  }
  await update($, viewAtom, () => view)
  // Open on the first changed section, else the whole spec.
  const first = view.changedSections[0]
  const at = first ? (spec.sectionByTitle(text, first)?.start ?? -1) : -1
  await update($, uiAtom, () => ({ file: '', shown: at, codeTab: 0 }))
  void $.ui.open({ id: PANE, title: 'Spec' })
  return { root, file, text }
}

async function codeView($: EngineInterface, path: string, start: number, end: number, label: string): Promise<CodeView> {
  const src: string = await $.fs.read(path)
  return { path, label, startLine: start, source: spec.sliceLines(src, start, end) }
}

// Resolves code refs the way the pane can: line ranges, and Go symbols by a
// regex over the file. The bridge's go/packages lookup is far more exact.
async function resolveCode($: EngineInterface, label: string, refs: check.CodeRefArg[], errs: string[]): Promise<CodeView[]> {
  const out: CodeView[] = []
  for (const r of refs) {
    if (!r.path) continue
    const p = abs(r.path)
    if (!(await $.fs.exists(p))) {
      errs.push(`${label}: code ref ${r.path} does not exist`)
      continue
    }
    if (r.lines) {
      const range = spec.parseLines(r.lines)
      if (!range) {
        errs.push(`${label}: code ref ${r.path}: lines ${r.lines} is not N or N-M`)
        continue
      }
      out.push(await codeView($, p, range.start, range.end, `${rel(p)}:${r.lines}`))
    } else if (r.symbol) {
      const range = spec.findGoSymbol(await $.fs.read(p), r.symbol)
      if (!range) {
        errs.push(`${label}: code ref ${r.path}: symbol ${r.symbol} not found`)
        continue
      }
      out.push(await codeView($, p, range.start, range.end, `${rel(p)} ${r.symbol}`))
    }
  }
  return out
}

/**
 * Resolves doc_refs into the spec's markdown files, as the bridge's
 * resolveDocs: a path relative to the folder of a folder spec, else to the
 * project directory; none means the primary file. Problems go to errs.
 */
async function resolveDocs($: EngineInterface, view: SpecView, label: string, refs: check.DocRefArg[], errs: string[]): Promise<DocRefView[]> {
  const out: DocRefView[] = []
  for (const d of refs) {
    const given = d.path ?? ''
    if (!given && view.specPath === '') {
      errs.push(`${label}: a doc ref without path means the primary spec file, and no spec is shown; give path, or call spec_show first`)
      continue
    }
    const full = expand(given)
    let p = view.specPath
    if (given) {
      p = full.startsWith('/') ? clean(full) : abs(given)
      // A relative path is looked up in the spec folder first, as the bridge does.
      if (!full.startsWith('/') && view.isDir && (await $.fs.exists(join(view.dir, given)))) p = clean(join(view.dir, given))
    }
    const md = MARKDOWN.test(p)
    if (!md && d.quote) {
      errs.push(`${label}: doc ref ${given} is not a markdown file; give lines instead of a quote`)
      continue
    }
    if (!(await $.fs.exists(p))) {
      errs.push(`${label}: doc ref ${given} does not exist`)
      continue
    }
    const text: string = p === view.specPath ? view.text : await $.fs.read(p)
    if (d.quote) {
      const { how: _how, ...q } = locate(text, d.quote, d.section ?? '', p, label, errs)
      if (q.quoteStart >= 0) out.push({ ...q, path: p, shownAt: sectionOf(text, q), startLine: 0, endLine: 0 })
      continue
    }
    const r = spec.parseLines(d.lines ?? '')
    const n = spec.lineCount(text)
    if (!r || r.start > n) {
      errs.push(`${label}: doc ref ${given} has ${n} lines; lines ${d.lines} are past the end`)
      continue
    }
    const endLine = Math.min(r.end, n)
    const { start, end } = spec.lineOffsets(text, r.start, endLine)
    const q: QuoteView = { quote: '', quoteStart: start, quoteEnd: end, quoteSection: d.section ?? '' }
    out.push({ ...q, path: p, shownAt: md ? sectionOf(text, q) : -1, startLine: r.start, endLine })
  }
  return out
}

/** Where the pane goes for a focus or a question: the primary quote, else the first doc ref. */
function targetOf(view: SpecView, q: QuoteView, docs: DocRefView[], section = ''): Omit<PaneUi, 'codeTab'> {
  if (q.quoteStart >= 0) return { file: '', shown: sectionOf(view.text, q) }
  const d = docs[0]
  if (d) return { file: d.path === view.specPath ? '' : d.path, shown: d.shownAt }
  return { file: '', shown: section ? (spec.sectionByTitle(view.text, section)?.start ?? -1) : -1 }
}

function scrollToQuote($: EngineInterface) {
  // Bring the focused block into view once the pane has drawn it.
  $.clock.after(250, () => void $.ui.scroll({ to: { key: 'quote' }, in: PANE, block: 'center' }).catch(() => undefined))
}

async function focus($: EngineInterface, args: Args) {
  const prev = await read($, viewAtom)
  if (!prev) throw new Error('focus: no spec is shown yet; call spec_show first')
  const errs = check.focus(args)
  if (errs.length > 0) throw new Error('invalid input:\n- ' + errs.join('\n- '))
  const text: string = await $.fs.read(prev.specPath)
  const section = String(args.spec_section ?? '')
  const q = locate(text, String(args.spec_quote ?? ''), section, prev.specPath, 'focus', errs)
  const docs = await resolveDocs($, { ...prev, text }, 'focus', (args.doc_refs as check.DocRefArg[] | undefined) ?? [], errs)

  const code = await resolveCode($, 'focus', (args.code_refs as check.CodeRefArg[] | undefined) ?? [], errs)
  // A bad ref fails the call, as the bridge's does.
  if (errs.length > 0) fail(errs)

  const { how, ...quote } = q
  const next = { ...prev, ...quote, text, docs, match: how, code, error: '' }
  await update($, viewAtom, () => next)
  await update($, uiAtom, () => ({ ...targetOf(next, quote, docs, section), codeTab: 0 }))
  void $.ui.open({ id: PANE, title: 'Spec' })
  if (q.quoteStart >= 0 || docs.length > 0) scrollToQuote($)
  return {
    ok: true,
    spec_match: how || undefined,
    docs: docs.map(d => ({ path: prev.isDir ? under(prev.dir, d.path) : rel(d.path), match: 'exact' })),
    code: code.map(c => ({ path: c.path, start_line: c.startLine })),
  }
}

// ---------------------------------------------------------------- questions

/** The view of ask mode with no spec open: no file, refs relative to the project directory. */
const noSpecView = (): SpecView => ({
  ...noQuote,
  specPath: '',
  title: '',
  dir: ctx.cwd,
  isDir: false,
  files: [],
  changedFiles: [],
  docs: [],
  text: '',
  changedSections: [],
  match: '',
  code: [],
  error: '',
  isDone: false,
  summary: '',
})

async function askMode($: EngineInterface, args: Args) {
  const on = args.off !== true
  await update($, modeAtom, () => on)
  await update($, releasedAtom, () => !on)
  const spec = on && args.spec === true
  const given = spec ? ((args.paths as string[] | undefined) ?? []).map(p => p.trim()).filter(p => p !== '') : []
  const paths: string[] = []
  for (const p of given) paths.push(await specPath($, p))
  await update($, workAtom, w =>
    on ? { ...w, spec, paths } : { ...noWork(), explicit: w.explicit, autoShown: w.autoShown },
  )
  if (on) {
    if (!(await read($, viewAtom))) await update($, viewAtom, () => noSpecView())
    void $.ui.open({ id: PANE, title: 'Spec' })
  } else {
    // The empty view only stood for ask mode, and a document picked from a
    // write only served it: clear both. A spec Claude showed stays.
    const picked = (await read($, workAtom)).autoShown
    await update($, viewAtom, v => (v?.specPath === '' || (v && v.specPath === picked) ? null : v))
    await update($, workAtom, w => ({ ...w, autoShown: '' }))
    await update($, uiAtom, () => ({ file: '', shown: -1, codeTab: 0 }))
  }
  if (!on) return { on, next: OFF_NEXT }
  if (!spec) return { on }
  const shown = await showExisting($, paths)
  return shown ? { on, spec, shown } : { on, spec }
}

/**
 * Shows a spec-mode path that names an existing markdown file, so a spec
 * written before spec mode started shows at once. Only a plain path counts,
 * not a glob. Returns the path shown, or ''.
 */
async function showExisting($: EngineInterface, paths: string[]): Promise<string> {
  const w = await read($, workAtom)
  if (w.explicit || (w.doc !== '' && !w.provisional)) return ''
  for (const raw of paths) {
    const p = clean(raw)
    if (/[*?[]/.test(p) || !MARKDOWN.test(p)) continue
    if (!(await $.fs.exists(p)) || (await $.fs.stat(p)).kind === 'dir') continue
    if (p === w.doc && !w.provisional) return p
    await showWorking($, p)
    return p
  }
  return ''
}

/** Whether p is an existing file, not a folder. */
async function isFile($: EngineInterface, p: string): Promise<boolean> {
  return (await $.fs.exists(p)) && (await $.fs.stat(p)).kind !== 'dir'
}

/** Reads a small file, or '' when it cannot. */
async function readSmall($: EngineInterface, p: string): Promise<string> {
  try {
    return (await $.fs.exists(p)) ? (await $.fs.read(p)).trim() : ''
  } catch {
    return ''
  }
}

/** The root of the git worktree that holds dir and its git folder, from the .git files, as the bridge's gitwt.Root. */
async function gitRoot($: EngineInterface, dir: string): Promise<{ root: string; gitDir: string } | null> {
  for (let d = clean(dir); ; d = dirOf(d)) {
    const p = join(d, '.git')
    if (await $.fs.exists(p)) {
      if ((await $.fs.stat(p)).kind === 'dir') return { root: d, gitDir: p }
      const m = /^gitdir:\s*(.+)$/.exec(await readSmall($, p))
      if (!m) return null
      return { root: d, gitDir: clean(m[1].startsWith('/') ? m[1] : join(d, m[1])) }
    }
    if (d === '/') return null
  }
}

/** The roots of every worktree of the repository that holds dir, the main one first, as the bridge's gitwt.Worktrees. */
async function worktrees($: EngineInterface, dir: string): Promise<string[]> {
  const r = await gitRoot($, dir)
  if (!r) return []
  let common = r.gitDir
  const c = await readSmall($, join(r.gitDir, 'commondir'))
  if (c) common = clean(c.startsWith('/') ? c : join(r.gitDir, c))
  const out: string[] = []
  const add = async (root: string) => {
    root = clean(root)
    if (!out.includes(root) && (await $.fs.exists(root)) && (await $.fs.stat(root)).kind === 'dir') out.push(root)
  }
  if (common.endsWith('/.git')) await add(dirOf(common))
  const meta = join(common, 'worktrees')
  if (await $.fs.exists(meta)) {
    for (const e of await $.fs.list(meta)) {
      const g = await readSmall($, join(join(meta, e.name), 'gitdir'))
      if (g.startsWith('/')) await add(dirOf(g))
    }
  }
  return out
}

/**
 * A spec-mode path made absolute, as the bridge's specPath: a relative path
 * with no glob that names no file under the session's folder is looked up in
 * the other worktrees of its repository, the most recently changed copy
 * first. With no copy anywhere it stays relative to the session's folder.
 */
async function specPath($: EngineInterface, p: string): Promise<string> {
  const full = clean(abs(p))
  const q = expand(p)
  if (q.startsWith('/') || /[*?[]/.test(q) || (await isFile($, full))) return full
  const r = await gitRoot($, ctx.cwd)
  if (!r) return full
  const sub = ctx.cwd.length > r.root.length ? ctx.cwd.slice(r.root.length + 1) : ''
  let best = ''
  let bestTime = -1
  for (const wt of await worktrees($, ctx.cwd)) {
    const c = clean(join(sub ? join(wt, sub) : wt, q))
    if (!(await isFile($, c))) continue
    const t = (await $.fs.stat(c)).mtimeMs ?? 0
    if (t > bestTime) {
      best = c
      bestTime = t
    }
  }
  return best || full
}

/** Shows a written file as the working document, as spec_show would. */
async function showWorking($: EngineInterface, path: string) {
  await showSpec($, { path })
  await update($, workAtom, w => ({ ...w, doc: path, autoShown: path, provisional: false }))
  $.ui.toast(`ai-contextview shows ${rel(path)} as the working document`)
}

/** A markdown file Claude wrote, in ask mode: maybe the working document (07). */
async function docWritten($: EngineInterface, path: string) {
  if (!(await read($, modeAtom))) return
  const patterns = ((await $.env.get('AI_CONTEXTVIEW_DOC_PATHS')) ?? '')
    .split(',')
    .map(p => p.trim())
    .filter(p => p !== '')
    .map(abs)
  if (!spec.countsAsDoc(path, ctx.home, patterns, ctx.cwd)) return
  let w = await read($, workAtom)
  if (w.explicit) return
  const edits = (w.edits[path] ?? 0) + 1
  await update($, workAtom, cur => ({ ...cur, edits: { ...cur.edits, [path]: edits } }))
  w = await read($, workAtom)
  // A provisional document waits for the first file that would be picked.
  if ((w.doc !== '' && !w.provisional) || path === w.doc) return
  let wants = true
  if (w.spec && w.paths.length > 0) wants = spec.matchAny(w.paths, path)
  else if (w.spec) {
    let text = ''
    try {
      text = await $.fs.read(path)
    } catch {
      text = ''
    }
    wants = spec.specScore(path, text, ctx.home, w.specSkill, edits) >= SCORE_THRESHOLD
  }
  if (wants) await showWorking($, path)
}

async function ask($: EngineInterface, args: Args, signal: AbortSignal): Promise<AskOutput> {
  const mode = await read($, modeAtom)
  const invalid = check.ask(args, mode)
  if (invalid.length > 0) throw new Error('invalid input:\n- ' + invalid.join('\n- '))
  // A question about another written file makes it the working document.
  const work = await read($, workAtom)
  if (mode && !work.explicit) {
    const asked = ((args.questions as check.QuestionArg[] | undefined) ?? [])
      .flatMap(q => q.doc_refs ?? [])
      .map(d => (d.path ? clean(abs(d.path)) : ''))
      .find(p => p !== '' && (work.edits[p] ?? 0) > 0 && p !== work.doc && MARKDOWN.test(p))
    if (asked) await showWorking($, asked)
  }
  let prev = await read($, viewAtom)
  if (!prev && mode) {
    prev = noSpecView()
    await update($, viewAtom, () => prev)
  }
  if (!prev) throw new Error('ask: no spec is shown yet; call spec_show first')
  if (prev.specPath === '' && (args.questions as check.QuestionArg[] | undefined)?.some(q => q.spec_quote)) {
    throw new Error('ask: no spec is shown, so spec_quote has nothing to quote; call spec_show first, or use doc_refs or code_refs')
  }
  const text: string = prev.specPath === '' ? '' : await $.fs.read(prev.specPath)

  const errs: string[] = []
  const questions: QuestionView[] = []
  for (const q of (args.questions as check.QuestionArg[]) ?? []) {
    const label = `question ${q.id}`
    const { how: _how, ...quote } = locate(text, q.spec_quote ?? '', q.spec_section ?? '', prev.specPath, label, errs)
    questions.push({
      ...quote,
      id: q.id ?? '',
      text: q.text ?? '',
      options: (q.options ?? []).map(check.normalizeRecommended),
      multi: !!q.multi_select,
      code: await resolveCode($, label, q.code_refs ?? [], errs),
      docs: await resolveDocs($, { ...prev, text }, label, q.doc_refs ?? [], errs),
    })
  }
  if (errs.length > 0) fail(errs)

  const qsId = `qs_${Math.random().toString(36).slice(2, 10)}`
  const first = questions[0]
  await update($, askAtom, () => ({
    qsId,
    title: String(args.title ?? ''),
    questions,
    index: 0,
    review: false,
    drafts: {},
    error: '',
    result: null,
  }))
  // Asking again reopens a finished spec.
  const opened = { ...prev, text, isDone: false }
  await update($, viewAtom, v => (v ? { ...v, text, isDone: false } : v))
  await update($, uiAtom, () => ({ ...(first ? targetOf(opened, first, first.docs) : { file: '', shown: -1 }), codeTab: FORM_TAB }))
  void $.ui.open({ id: PANE, title: 'Spec' })
  $.ui.toast(`Claude asks ${questions.length === 1 ? 'a question' : `${questions.length} questions`} in the ai-contextview pane`)
  if (first && (first.quoteStart >= 0 || first.docs.length > 0)) scrollToQuote($)
  return waitFor($, qsId, signal)
}

/** Waits for the person's choice, up to ASK_WAIT_MS, then answers pending. */
async function waitFor($: EngineInterface, qsId: string, signal: AbortSignal): Promise<AskOutput> {
  const start = await $.clock.now()
  for (;;) {
    const got = ctx.results.get(qsId)
    if (got) return got
    if ((await $.clock.now()) - start >= ASK_WAIT_MS) {
      return { status: 'pending', question_set_id: qsId, message: MSG_PENDING }
    }
    if (signal.aborted) throw new Error('interrupted')
    await $.process.run(PAUSE, { timeoutMs: 10_000 })
  }
}

async function askWait($: EngineInterface, args: Args, signal: AbortSignal): Promise<AskOutput> {
  const id = String(args.question_set_id ?? '')
  const got = ctx.results.get(id)
  if (got) return got
  // A result the person chose before a reload of the mod is kept in state.
  const pending = await read($, askAtom)
  if (pending?.qsId === id) return pending.result ?? waitFor($, id, signal)
  throw new Error(`unknown question_set_id "${id}"`)
}

async function specDone($: EngineInterface, args: Args) {
  const summary = String(args.summary ?? '').trim()
  const n = check.utf8Length(summary)
  if (n > check.MAX_SUMMARY) throw new Error(`done: summary is ${n} bytes, at most ${check.MAX_SUMMARY}`)
  await update($, viewAtom, v => (v ? { ...v, ...noQuote, match: '', code: [], docs: [], isDone: true, summary } : v))
  await update($, uiAtom, () => ({ file: '', shown: -1, codeTab: 0 }))
  void $.ui.open({ id: PANE, title: 'Spec' })
  return { ok: true }
}

/** Ends a question set with the person's choice and wakes the hook waiting on it. */
async function settle($: EngineInterface, a: AskView, out: AskOutput) {
  ctx.results.set(a.qsId, out)
  await update($, askAtom, cur => (cur?.qsId === a.qsId ? { ...cur, result: out, error: '' } : cur))
}

function answersOf(a: AskView): { answers: Answer[]; deferred: string[]; missing: string[] } {
  const answers: Answer[] = []
  const deferred: string[] = []
  const missing: string[] = []
  for (const q of a.questions) {
    const d = a.drafts[q.id] ?? emptyDraft()
    const notes = d.notes.trim() || null
    if (d.later) {
      deferred.push(q.id)
      answers.push({ id: q.id, selected: [], other: null, notes, deferred: true })
      continue
    }
    const valid = new Set(q.options.map(o => o.label))
    let selected = d.selected.filter(s => valid.has(s))
    if (!q.multi) selected = selected.slice(0, 1)
    const other = d.other.trim() ? d.other.trim() : null
    if (selected.length === 0 && other === null) missing.push(q.id)
    answers.push({ id: q.id, selected, other, notes })
  }
  return { answers, deferred, missing }
}

const isAnswered = (q: QuestionView, d: Draft | undefined) =>
  !!d && (d.later || d.selected.length > 0 || d.other.trim() !== '')

async function gotoQuestion($: EngineInterface, k: number) {
  const a = await read($, askAtom)
  const q = a?.questions[k]
  if (!a || !q) return
  await update($, askAtom, cur => (cur ? { ...cur, index: k, review: false } : cur))
  const v = await read($, viewAtom)
  if (q.quoteStart < 0 && q.docs.length === 0) {
    // No document: keep the one shown, so the pane does not jump.
    await update($, uiAtom, u => ({ ...u, codeTab: FORM_TAB }))
    return
  }
  await update($, uiAtom, () => ({ ...(v ? targetOf(v, q, q.docs) : { file: '', shown: -1 }), codeTab: FORM_TAB }))
  scrollToQuote($)
}

async function editDraft($: EngineInterface, id: string, fn: (d: Draft) => Draft) {
  await update($, askAtom, cur => (cur ? { ...cur, error: '', drafts: { ...cur.drafts, [id]: fn(cur.drafts[id] ?? emptyDraft()) } } : cur))
}

/** The next unanswered question after index k, wrapping around; -1 when all have an answer. */
function nextOpen(a: AskView, k: number): number {
  const n = a.questions.length
  for (let step = 1; step <= n; step++) {
    const i = (k + step) % n
    const q = a.questions[i]!
    if (!isAnswered(q, a.drafts[q.id])) return i
  }
  return -1
}

/** Shows every answer for review, as the browser page does after the last one. */
async function showReview($: EngineInterface) {
  await update($, askAtom, cur => (cur ? { ...cur, review: true, error: '' } : cur))
  const v = await read($, viewAtom)
  await update($, uiAtom, () => ({ ...(v ? targetOf(v, v, v.docs) : { file: '', shown: -1 }), codeTab: FORM_TAB }))
}

/** After a single choice or Decide later: the next open question, or the review. */
async function advance($: EngineInterface, q: QuestionView) {
  const a = await read($, askAtom)
  if (!a || a.questions[a.index]?.id !== q.id || !isAnswered(q, a.drafts[q.id])) return
  const next = nextOpen(a, a.index)
  if (next >= 0) await gotoQuestion($, next)
  else await showReview($)
}

async function pickOption($: EngineInterface, q: QuestionView, label: string) {
  await editDraft($, q.id, d => {
    if (!q.multi) return { ...d, selected: [label], later: false }
    const has = d.selected.includes(label)
    return { ...d, selected: has ? d.selected.filter(s => s !== label) : [...d.selected, label], later: false }
  })
  if (!q.multi) await advance($, q)
}

async function toggleLater($: EngineInterface, q: QuestionView) {
  let later = false
  await editDraft($, q.id, cur => {
    later = !cur.later
    return later ? { ...cur, later: true, selected: [], other: '' } : { ...cur, later: false }
  })
  if (later) await advance($, q)
}

/** One answer as text for the review, as answerText in the browser page. */
function answerText(q: QuestionView, d: Draft): string {
  if (d.later) return 'Decide later'
  const parts = q.options.map(o => o.label).filter(l => d.selected.includes(l))
  if (d.other.trim()) parts.push(`Other: ${d.other.trim()}`)
  return parts.join(', ')
}

async function sendAnswers($: EngineInterface) {
  const a = await read($, askAtom)
  if (!a || a.result) return
  const { answers, deferred, missing } = answersOf(a)
  if (missing.length > 0) {
    await update($, askAtom, cur => (cur ? { ...cur, error: `Answer ${missing.join(', ')}, or choose Decide later.` } : cur))
    await gotoQuestion($, a.questions.findIndex(q => q.id === missing[0]))
    return
  }
  const out: AskOutput = { status: 'answered', answers }
  if (deferred.length > 0) {
    out.deferred = deferred
    out.message = MSG_DEFERRED
  }
  await settle($, a, out)
}

/** The questions as plain text, for the chat: "Back to chat" copies it. */
function clipboardText(a: AskView): string {
  const lines: string[] = []
  if (a.title) lines.push(a.title, '')
  a.questions.forEach((q, i) => {
    lines.push(`${i + 1}. ${q.text.trim()}`)
    q.options.forEach((o, k) => lines.push(`   ${String.fromCharCode(97 + k)}) ${o.label}${o.description ? ` (${o.description})` : ''}`))
    lines.push('')
  })
  return lines.join('\n').trim() + '\n'
}

async function backToChat($: EngineInterface, surface: Parameters<EngineInterface['ui']['copy']>[0]['surface']) {
  const a = await read($, askAtom)
  if (!a || a.result) return
  const copied = await $.ui.copy({ text: clipboardText(a), surface })
  $.ui.toast(copied.isCopied ? 'Questions copied to the clipboard' : 'Could not copy the questions')
  await settle($, a, { status: 'in_chat', message: MSG_IN_CHAT })
}

async function skip($: EngineInterface) {
  const a = await read($, askAtom)
  if (!a || a.result) return
  await settle($, a, { status: 'dismissed', message: MSG_DISMISSED })
}

// ------------------------------------------------------------------ register

export const register: Register = on => {
  on('session.start', async ($, e, next) => {
    ctx.cwd = e.cwd
    ctx.home = (await $.env.get('HOME')) ?? (await $.env.get('USERPROFILE')) ?? ''
    await $.command.register({ name: 'contextview-pane', description: 'Open the ai-contextview pane' })
    for (const t of TOOLS) await $.tool.register(t)
    // AI_CONTEXTVIEW_ASK_MODE=always turns ask mode on for every session.
    if ((await $.env.get('AI_CONTEXTVIEW_ASK_MODE')) === 'always' && !(await read($, modeAtom))) {
      await update($, modeAtom, () => true)
      if (!(await read($, viewAtom))) await update($, viewAtom, () => noSpecView())
    }
    if (await read($, viewAtom)) void $.ui.open({ id: PANE, title: 'Spec' })
    return next(e)
  })

  on('command.run', { command: 'contextview-pane' }, async $ => {
    await $.ui.open({ id: PANE, title: 'Spec' })
    return { text: 'ai-contextview pane opened.' }
  })

  on('tool.call', async ($, e, next) => {
    // ai-contextview's hook asks the daemon, which the pane does not use, so the
    // mod blocks the built-in question tool itself while a spec is open here.
    if (e.tool === 'AskUserQuestion') {
      if (await read($, releasedAtom)) return next(e)
      const v = await read($, viewAtom)
      // A file the pane shows because a write picked it is not a spec Claude opened.
      const picked = (await read($, workAtom)).autoShown
      if (v && v.specPath !== '' && !v.isDone && v.specPath !== picked) return { deny: BLOCK_REASON }
      if (await read($, modeAtom)) return { deny: BLOCK_REASON_ASK_MODE }
      return next(e)
    }

    if (e.tool === 'Skill') {
      const skill = String((e as unknown as Args).skill ?? '')
      if (SPEC_SKILL.test(skill)) await update($, workAtom, w => ({ ...w, specSkill: true }))
      return next(e)
    }
    if (WRITE_TOOLS.has(e.tool)) {
      const res = await next(e)
      const given = String((e as unknown as Args).file_path ?? '')
      if (given) await docWritten($, clean(abs(given)))
      return res
    }

    const prefix = `mcp__${$.plugin.name}__`
    const name = e.tool.startsWith(prefix) ? e.tool.slice(prefix.length) : ''
    if (!TOOL_NAMES.has(name)) return next(e)
    const args = e as unknown as Args

    // The mod's own tools: an answer here serves the call. An MCP tool's
    // result is text, so each answer is the JSON the bridge would send.
    try {
      let out: unknown
      if (name === 'spec_show') {
        const provisional = args.provisional === true
        if (provisional && !(await read($, modeAtom))) {
          throw new Error(
            'provisional is only allowed in ask mode or spec mode; without it, call spec_show without provisional',
          )
        }
        const { root, file, text } = await showSpec($, args)
        // Claude's own spec_show wins over the document picked from writes. A
        // provisional one is shown like a picked one: a later write replaces it.
        await update($, workAtom, w =>
          provisional
            ? { ...w, explicit: false, doc: file, autoShown: file, provisional: true }
            : { ...w, explicit: true, autoShown: '', provisional: false },
        )
        const artifacts = []
        for (const f of root.files) {
          const body: string = f === file ? text : await $.fs.read(f)
          artifacts.push({ path: under(root.dir, f), kind: 'other', sections: spec.headings(body).map(h => h.title) })
        }
        out = { ok: true, primary: under(root.dir, file), sections: spec.headings(text).map(h => h.title), artifacts }
      } else if (name === 'focus') {
        out = await focus($, args)
      } else if (name === 'ask') {
        out = await ask($, args, next.signal)
      } else if (name === 'ask_mode') {
        out = await askMode($, args)
      } else if (name === 'ask_wait') {
        out = await askWait($, args, next.signal)
      } else {
        out = await specDone($, args)
      }
      return { result: JSON.stringify(out) }
    } catch (err) {
      return { deny: (err as Error).message }
    }
  }).catch(($, e, next) => {
    if (next.called) return next(e)
    // A call of the mod's own tools has nothing beneath to go to.
    if (e.tool.startsWith(`mcp__${$.plugin.name}__`)) {
      return { deny: 'ai-contextview pane: the call was interrupted or failed. Call it again.' }
    }
    $.ui.status('ai-contextview pane: a hook failed; the call went on without it')
    return next(e)
  })

  on('ui.render', { component: 'Pane', requestId: PANE }, async ($, e) => {
    const { Box, Text, Markdown, Code, Select, Button, Input } = $.ui.resolve(e)
    const view = await read($, viewAtom)
    const ui = await read($, uiAtom)
    const a = await read($, askAtom)
    if (!view) return <Text dimColor>No spec shown yet.</Text>
    const work = await read($, workAtom)
    const provisional = work.provisional && work.doc === view.specPath

    const asking = a && !a.result ? a : null
    const reviewing = !!asking?.review
    const question = asking && !reviewing ? asking.questions[asking.index] : undefined
    // What the spec and code columns show: the current question's refs while
    // questions wait, else the last focus.
    const at: QuoteView = question ?? view
    const codes = question ? question.code : view.code
    const docs = question ? question.docs : view.docs

    // The file shown: the primary, or another file of the spec.
    const primary = view.specPath
    const shownFile = ui.file || primary
    let text = view.text
    let fileError = ''
    if (shownFile !== primary) {
      try {
        text = await $.fs.read(shownFile)
      } catch (err) {
        text = ''
        fileError = `Cannot read ${shownFile}: ${(err as Error).message}`
      }
    }
    // The quote framed in that file, if it holds one.
    const quoted: QuoteView | undefined =
      shownFile === primary && at.quoteStart >= 0 ? at : docs.find(d => d.path === shownFile)
    const target = targetOf(view, at, docs)
    const label = (p: string) => (view.isDir ? under(view.dir, p) : rel(p))

    // A document that is not markdown is shown as source, without sections.
    const source = shownFile !== '' && !MARKDOWN.test(shownFile)
    const heads = source ? [] : spec.headings(text)
    const section = !source && ui.shown >= 0 ? spec.sectionAt(text, ui.shown) : null
    const lo = section?.start ?? 0
    const hi = section?.end ?? text.length
    const shownText = text.slice(lo, hi)
    const parts =
      quoted && quoted.quoteStart >= lo && quoted.quoteEnd <= hi
        ? source
          ? {
              before: shownText.slice(0, quoted.quoteStart - lo),
              focus: shownText.slice(quoted.quoteStart - lo, quoted.quoteEnd - lo),
              after: shownText.slice(quoted.quoteEnd - lo),
            }
          : spec.splitAround(shownText, quoted.quoteStart - lo, quoted.quoteEnd - lo)
        : { before: shownText, focus: '', after: '' }
    // First line numbers of the source parts.
    const focusLine = spec.lineCount(parts.before) + 1
    const afterLine = focusLine + spec.lineCount(parts.focus)
    const wide = e.props.placement === 'dock' && e.props.bodyColumns >= WIDE
    const formShown = !!asking && (question !== undefined || reviewing)
    // Two columns while questions wait: the side column shows the form or one
    // code ref, chosen by tabs, so neither pushes the other out of view.
    const sideTabs = wide && formShown && codes.length > 0
    const onForm = sideTabs && ui.codeTab < 0
    const tab = Math.min(Math.max(0, ui.codeTab), Math.max(0, codes.length - 1))
    const active = codes[tab]
    const showSection = (where: number) => update($, uiAtom, u => ({ ...u, shown: where }))
    const showFile = (file: string) => {
      const d = docs.find(one => one.path === file)
      const atQuote = file === primary ? (at.quoteStart >= 0 ? sectionOf(view.text, at) : -1) : (d?.shownAt ?? -1)
      return update($, uiAtom, u => ({ ...u, file: file === primary ? '' : file, shown: atQuote }))
    }
    const hasTarget = at.quoteStart >= 0 || docs.length > 0
    const awayFromTarget = (ui.file || '') !== target.file || (ui.shown !== -1 && ui.shown !== target.shown)

    const options = [
      { value: '-1', label: shownFile === primary ? 'Whole spec' : 'Whole file' },
      ...heads.map(h => ({
        value: String(h.offset),
        label: `${'  '.repeat(h.level - 1)}${h.title}${shownFile === primary && view.changedSections.includes(h.title) ? '  (changed)' : ''}`,
      })),
    ]
    // Every file of the spec, and any other file a doc ref quotes.
    const files = [...view.files, ...docs.map(d => d.path).filter(p => !view.files.includes(p))].filter(
      (p, i, all) => all.indexOf(p) === i,
    )
    const fileOptions = files.map(p => ({
      value: p,
      label: `${label(p)}${view.changedFiles.includes(p) ? '  (changed)' : ''}${docs.some(d => d.path === p) || (p === primary && at.quoteStart >= 0) ? '  (quoted)' : ''}`,
    }))

    // Ask mode with no spec open: no file to show.
    const noSpec = view.specPath === '' && shownFile === ''
    // With no spec, the document column shows for a whole question set when
    // any of its questions has a document, so it does not come and go.
    const setHasDocs = !!asking && asking.questions.some(q => q.docs.length > 0)
    const hideDocs = view.specPath === '' && !!asking && !setHasDocs
    const noDocNote = !!question && !hideDocs && question.docs.length === 0 && question.quoteStart < 0
    const specColumn = hideDocs ? null : (
      <Box key="spec-col" flexDirection="column" width={wide && (formShown || codes.length > 0) ? '55%' : '100%'} flexShrink={1}>
        {noSpec && (
          <Text key="no-spec" dimColor wrap="wrap">
            Ask mode is on: Claude asks every question of this session here. No spec is open.
          </Text>
        )}
        {noDocNote && (
          <Text key="no-doc" color="yellow" bold>
            This question has no document
          </Text>
        )}
        {source && parts.before !== '' && (
          <Code key="before-src" source={parts.before.slice(-MAX_CODE)} path={shownFile} startLine={1} />
        )}
        {!source && parts.before !== '' && <Markdown key="before" text={parts.before.slice(-MAX_MARKDOWN)} />}
        {parts.focus !== '' && (
          <Box key="quote" flexDirection="column" borderStyle="round" borderColor="yellow" paddingX={1}>
            <Text key="quote-label" color="yellow" size="caption" wrap="truncate-end" maxLines={1}>
              {`${question ? `Question ${asking!.index + 1}` : 'Focus'}${
                quoted?.quoteSection && section?.title !== quoted.quoteSection ? ` in ${quoted.quoteSection}` : ''
              }${question ? `: ${oneLine(question.text)}` : ''}`}
            </Text>
            {source ? (
              <Code key="quote-src" source={parts.focus} path={shownFile} startLine={focusLine} />
            ) : (
              <Markdown key="quote-text" text={parts.focus} />
            )}
          </Box>
        )}
        {source && parts.after !== '' && (
          <Code key="after-src" source={parts.after.slice(0, MAX_CODE)} path={shownFile} startLine={afterLine} />
        )}
        {!source && parts.after !== '' && <Markdown key="after" text={parts.after.slice(0, MAX_MARKDOWN)} />}
      </Box>
    )

    const codeBlock =
      codes.length === 0 ? null : (
        <Box key="code" flexDirection="column" rowGap={1}>
          <Box flexDirection="row" flexWrap="wrap" columnGap={1}>
            {sideTabs && (
              <Button
                key="side-form"
                variant={onForm ? 'primary' : 'secondary'}
                onPress={() => update($, uiAtom, u => ({ ...u, codeTab: FORM_TAB }))}
              >
                {reviewing ? 'Answers' : 'Question'}
              </Button>
            )}
            {codes.map((c, i) => (
              <Button
                key={`code-${i}`}
                variant={!onForm && i === tab ? 'primary' : 'secondary'}
                onPress={() => update($, uiAtom, u => ({ ...u, codeTab: i }))}
              >
                {c.label.replace(/^.*\//, '')}
              </Button>
            ))}
          </Box>
          {active && !onForm && (
            <Box flexDirection="column">
              <Text dimColor>{active.label}</Text>
              <Code source={active.source.slice(0, MAX_CODE)} path={active.path} startLine={active.startLine} />
            </Box>
          )}
        </Box>
      )

    let askPanel = null
    if (asking && (question || reviewing)) {
      const n = asking.questions.length
      const all = asking.questions.every(q => isAnswered(q, asking.drafts[q.id]))
      let body
      if (question) {
        const d = asking.drafts[question.id] ?? emptyDraft()
        body = (
          <Box key={`question-${question.id}`} flexDirection="column" rowGap={1}>
            <Markdown key="q-text" text={question.text} />
            {noDocNote && (
              <Text key="q-nodoc" color="yellow">
                No document
              </Text>
            )}
            {!wide && question.quote !== '' && (
              // One column puts the spec under the form: repeat the quote here.
              <Box key="q-quote" paddingLeft={2}>
                <Text dimColor italic wrap="wrap" maxLines={3}>
                  {`“${oneLine(question.quote)}”`}
                </Text>
              </Box>
            )}
            <Box flexDirection="column">
              {question.options.map((o, k) => {
                const picked = !d.later && d.selected.includes(o.label)
                // A desktop draws a Button on one row and cuts what does not fit,
                // so the description goes under it as Text, which wraps.
                return (
                  <Box key={`opt-box-${k}`} flexDirection="column">
                    <Button
                      key={`opt-${k}`}
                      variant={picked ? 'primary' : 'secondary'}
                      onPress={() => pickOption($, question, o.label)}
                    >
                      {`${question.multi ? (picked ? '☑' : '☐') : picked ? '●' : '○'} ${o.label}`}
                      {o.recommended && (
                        <Text key={`opt-rec-${k}`} color="green" bold>
                          {'  recommended'}
                        </Text>
                      )}
                    </Button>
                    {o.description !== '' && (
                      <Box key={`opt-desc-${k}`} paddingLeft={2}>
                        <Text dimColor wrap="wrap">
                          {o.description}
                        </Text>
                      </Box>
                    )}
                  </Box>
                )
              })}
              {!question.options.some(o => spec.isLaterLabel(o.label)) && (
                // The question already offers its own "decide later" option.
                <Button
                  key="later"
                  variant={d.later ? 'primary' : 'secondary'}
                  onPress={() => toggleLater($, question)}
                >
                  {`${d.later ? '●' : '○'} Decide later`}
                </Button>
              )}
            </Box>
            <Input
              key={`other-${question.id}`}
              label="Other"
              placeholder="Your own answer"
              value={d.other}
              onInput={value => void editDraft($, question.id, cur => ({ ...cur, other: value, later: value.trim() ? false : cur.later }))}
              onSubmit={value => void editDraft($, question.id, cur => ({ ...cur, other: value }))}
            />
            <Input
              key={`notes-${question.id}`}
              label="Notes"
              placeholder="Optional, sent with the answer"
              value={d.notes}
              onInput={value => void editDraft($, question.id, cur => ({ ...cur, notes: value }))}
              onSubmit={value => void editDraft($, question.id, cur => ({ ...cur, notes: value }))}
            />
          </Box>
        )
      } else {
        body = (
          <Box key="review" flexDirection="column" rowGap={1}>
            <Text>All questions answered. Review and send, or press a question number to change it.</Text>
            {asking.questions.map((q, k) => {
              const d = asking.drafts[q.id] ?? emptyDraft()
              const notes = d.notes.trim()
              return (
                <Box key={`review-box-${k}`} flexDirection="column">
                  <Box flexDirection="row" columnGap={1}>
                    <Button key={`review-${k}`} plain onPress={() => gotoQuestion($, k)}>
                      {`Q${k + 1}`}
                    </Button>
                    <Text dimColor wrap="wrap">{q.text.trim()}</Text>
                  </Box>
                  <Box paddingLeft={2} flexDirection="column">
                    <Text color={d.later ? 'yellow' : 'cyan'} bold wrap="wrap">
                      {`→ ${answerText(q, d)}`}
                    </Text>
                    {notes !== '' && (
                      <Box paddingLeft={2}>
                        <Text dimColor italic wrap="wrap">{`Notes: ${notes}`}</Text>
                      </Box>
                    )}
                  </Box>
                </Box>
              )
            })}
          </Box>
        )
      }
      askPanel = (
        <Box key="ask" flexDirection="column" borderStyle="round" borderColor="cyan" paddingX={1} rowGap={1}>
          <Box flexDirection="row" flexWrap="wrap" columnGap={1} alignItems="center">
            <Text bold>{asking.title || 'Questions'}</Text>
            {asking.questions.map((q, k) => (
              <Button
                key={`step-${k}`}
                plain
                variant={k === asking.index && !reviewing ? 'primary' : 'secondary'}
                onPress={() => gotoQuestion($, k)}
              >
                {`${k + 1}${isAnswered(q, asking.drafts[q.id]) ? ' ✓' : ''}`}
              </Button>
            ))}
          </Box>
          {body}
          {asking.error !== '' && <Text color="red">{asking.error}</Text>}
          {(reviewing || n > 1 || all) && (
          <Box key="ask-nav" flexDirection="row" flexWrap="wrap" columnGap={1}>
            {reviewing ? (
              <Button key="prev" onPress={() => gotoQuestion($, n - 1)}>
                Previous
              </Button>
            ) : (
              asking.index > 0 && (
                <Button key="prev" onPress={() => gotoQuestion($, asking.index - 1)}>
                  Previous
                </Button>
              )
            )}
            {!reviewing && asking.index < n - 1 && (
              <Button key="next" onPress={() => gotoQuestion($, asking.index + 1)}>
                Next
              </Button>
            )}
            {!reviewing && asking.index === n - 1 && all && (
              <Button key="to-review" onPress={() => showReview($)}>
                Review
              </Button>
            )}
          </Box>
          )}
          <Box key="ask-end" flexDirection="row" flexWrap="wrap" columnGap={2} alignItems="center">
            <Button key="send" variant={all ? 'primary' : 'secondary'} onPress={() => sendAnswers($)}>
              Send answers
            </Button>
            <Button key="in-chat" plain dimColor onPress={press => backToChat($, press.surface)}>
              Back to chat
            </Button>
            <Button key="skip" plain dimColor onPress={() => skip($)}>
              Skip
            </Button>
          </Box>
        </Box>
      )
    }

    // With side tabs, the tab row comes first and the form takes the code's place.
    const side =
      askPanel || codeBlock ? (
        <Box key="side-col" flexDirection="column" width={wide && specColumn ? '45%' : '100%'} flexShrink={1} rowGap={1}>
          {sideTabs ? codeBlock : askPanel}
          {sideTabs ? onForm && askPanel : codeBlock}
        </Box>
      ) : null

    // One line: the file, then the section shown, the spec it belongs to and the match.
    const crumbs = [
      section ? `› ${section.title}` : `› ${shownFile === primary ? 'Whole spec' : 'Whole file'}`,
      shownFile !== primary && view.title !== '' ? `in the spec ${view.title}` : '',
      view.match !== '' && !question && shownFile === primary ? `quote match ${view.match}` : '',
    ].filter(s => s !== '')
    const jumps =
      (shownFile === primary && view.changedSections.length > 0) ||
      docs.some(d => d.path !== shownFile) ||
      (hasTarget && awayFromTarget)

    return (
      <Box flexDirection="column" rowGap={1}>
        <Box flexDirection="column">
          <Box key="crumbs" flexDirection="row" flexWrap="wrap" columnGap={1}>
            <Text bold>{noSpec ? 'Ask mode' : label(shownFile)}</Text>
            {!noSpec && provisional && (
              <Text key="provisional" color="yellow">
                (provisional, until the working document is written)
              </Text>
            )}
            {!noSpec && crumbs.map((c, i) => (
              <Text key={`crumb-${i}`} dimColor>
                {c}
              </Text>
            ))}
          </Box>
          {view.error !== '' && <Text color="red">{view.error}</Text>}
          {fileError !== '' && <Text color="red">{fileError}</Text>}
        </Box>
        {view.isDone && (
          <Box key="done" flexDirection="column" borderStyle="round" borderColor="green" paddingX={1}>
            <Text color="green" bold>
              Spec finished
            </Text>
            {view.summary !== '' && <Markdown key="summary" text={view.summary} />}
          </Box>
        )}
        <Box flexDirection="row" flexWrap="wrap" columnGap={2} alignItems="center">
          {files.length > 1 && (
            <Select key="file" label="File" options={fileOptions} value={shownFile} onSelect={v => void showFile(v)} />
          )}
          {!noSpec && (
            <Select
              key="outline"
              label="Section"
              options={options}
              value={String(ui.shown)}
              onSelect={v => void showSection(Number(v))}
            />
          )}
        </Box>
        {jumps && (
        <Box key="jumps" flexDirection="row" flexWrap="wrap" columnGap={2} alignItems="center">
          {shownFile === primary && view.changedSections.length > 0 && <Text dimColor>Changed:</Text>}
          {shownFile === primary &&
            view.changedSections.map((title, i) => (
              <Button key={`changed-${i}`} plain onPress={() => showSection(spec.sectionByTitle(text, title)?.start ?? -1)}>
                {title}
              </Button>
            ))}
          {docs
            .filter(d => d.path !== shownFile)
            .map((d, i) => (
              <Button key={`doc-${i}`} plain onPress={() => showFile(d.path)}>
                {`Quote in ${label(d.path)}`}
              </Button>
            ))}
          {hasTarget && awayFromTarget && (
            <Button
              key="to-quote"
              plain
              onPress={() => update($, uiAtom, u => ({ ...u, file: target.file, shown: target.shown }))}
            >
              {question ? 'Back to the question' : 'Back to the focus'}
            </Button>
          )}
        </Box>
        )}
        {wide ? (
          <Box flexDirection="row" columnGap={2}>
            {specColumn}
            {side}
          </Box>
        ) : (
          <Box flexDirection="column" rowGap={1}>
            {askPanel}
            {specColumn}
            {codeBlock}
          </Box>
        )}
      </Box>
    )
  })
}
