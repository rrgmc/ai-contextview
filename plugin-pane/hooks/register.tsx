// ai-specview-pane: draws the ai-specview spec, code and questions in a pane
// beside the chat.
//
// It hooks the ai-specview MCP tools by name, so the model, the skill and the
// browser viewer stay as they are. The mode picks who serves them:
//
//   browser  the MCP bridge alone (the mod does nothing)
//   both     the bridge serves every tool; the pane mirrors spec_show and
//            focus, using the bridge's resolved code ranges. Questions are
//            answered in the browser.
//   pane     the mod serves every tool itself and the daemon is not used:
//            questions are answered in the pane, and the built-in
//            AskUserQuestion is blocked while a spec is open.

import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register } from 'claude-code'

import type {
  Answer,
  AskOutput,
  AskView,
  CodeView,
  Draft,
  Mode,
  PaneUi,
  QuestionView,
  QuoteView,
  SpecView,
} from '../types'
import * as spec from './spec'
import * as check from './validate'

const PANE = 'ai-specview'
const MODES: readonly Mode[] = ['both', 'pane', 'browser']
const TOOL = /^mcp__.*ai-specview__(spec_show|focus|ask|ask_wait|spec_done)$/

// How long ask and ask_wait wait before answering pending, as the bridge's
// AI_SPECVIEW_ASK_WAIT default. A tool.call hook may run 20 minutes.
const ASK_WAIT_MS = 10 * 60_000

// A hook's own time is capped at 10 seconds and a $.clock wait counts toward
// it, but time inside any other $ call does not (up to 20 minutes on
// tool.call). So the wait pauses between checks with `sleep 1` run through
// $.process.run.
const PAUSE = ['/bin/sleep', '1'] as const

// Change a tag when the stored shape changes: it declines an older value.
const viewAtom = atom({ plugin: 'ai-specview-pane', key: 'view' } as const, null as SpecView | null, {
  shape: 'view-3',
})
const modeAtom = atom({ plugin: 'ai-specview-pane', key: 'mode' } as const, null as Mode | null, {
  shape: 'mode-1',
})
const uiAtom = atom({ plugin: 'ai-specview-pane', key: 'ui' } as const, { shown: -1, codeTab: 0 } as PaneUi, {
  shape: 'ui-1',
})
const askAtom = atom({ plugin: 'ai-specview-pane', key: 'ask' } as const, null as AskView | null, {
  shape: 'ask-1',
})

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
  'A spec is open in ai-specview for this session. Ask this with the ai-specview `ask` tool ' +
  'instead of AskUserQuestion: quote the spec verbatim with spec_quote and ' +
  'spec_section, and point at the code with code_refs.'

type Args = Record<string, unknown>
type ResolvedRef = { path: string; start_line: number; end_line: number; symbol?: string }

const MAX_MARKDOWN = 50_000
const MAX_CODE = 40_000
// The pane splits into two columns from this many cells across.
const WIDE = 100

type Ctx = {
  configured: Mode
  cwd: string
  // Outcomes the person chose, by question set, for the hook that waits on it.
  results: Map<string, AskOutput>
}

// Module state: it starts over on a reload, and session.start sets cwd again.
const ctx: Ctx = { configured: 'both', cwd: '', results: new Map() }

const abs = (p: string) => (p.startsWith('/') ? p : `${ctx.cwd.replace(/\/$/, '')}/${p}`)
const rel = (p: string) => (ctx.cwd && p.startsWith(ctx.cwd + '/') ? p.slice(ctx.cwd.length + 1) : p)
const emptyDraft = (): Draft => ({ selected: [], other: '', notes: '', later: false })
const noQuote: QuoteView = { quote: '', quoteStart: -1, quoteEnd: -1, quoteSection: '' }

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

async function modeOf($: EngineInterface): Promise<Mode> {
  return (await read($, modeAtom)) ?? ctx.configured
}

async function primaryFile($: EngineInterface, path: string, primary?: string): Promise<string> {
  const p = abs(path)
  const st = await $.fs.stat(p)
  if (st.kind !== 'dir') return p
  if (primary) return `${p}/${primary}`
  const md = (await $.fs.list(p))
    .map((f: { name: string }) => f.name)
    .filter((n: string) => n.toLowerCase().endsWith('.md'))
    .sort()
  const pick = md.find((n: string) => /^(readme|spec)\.md$/i.test(n)) ?? md[0]
  if (!pick) throw new Error(`${path}: the folder holds no markdown file`)
  return `${p}/${pick}`
}

async function showSpec($: EngineInterface, args: Args, via: SpecView['via']) {
  const file = await primaryFile($, String(args.path ?? ''), args.primary as string | undefined)
  const text: string = await $.fs.read(file)
  const view: SpecView = {
    ...noQuote,
    specPath: file,
    title: rel(file),
    text,
    changedSections: (args.changed_sections as string[] | undefined) ?? [],
    match: '',
    code: [],
    via,
    error: '',
    isDone: false,
    summary: '',
  }
  await update($, viewAtom, () => view)
  // Open on the first changed section, else the whole spec.
  const changed = view.changedSections[0]
  const at = changed ? (spec.sectionByTitle(text, changed)?.start ?? -1) : -1
  await update($, uiAtom, () => ({ shown: at, codeTab: 0 }))
  void $.ui.open({ id: PANE, title: 'Spec' })
  return { file, text }
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

function scrollToQuote($: EngineInterface) {
  // Bring the focused block into view once the pane has drawn it.
  $.clock.after(250, () => void $.ui.scroll({ to: { key: 'quote' }, in: PANE, block: 'center' }).catch(() => undefined))
}

async function focus($: EngineInterface, args: Args, via: SpecView['via'], resolved: ResolvedRef[] | null) {
  const prev = await read($, viewAtom)
  if (!prev) throw new Error('focus: no spec is shown yet; call spec_show first')
  const errs = via === 'pane' ? check.focus(args) : []
  if (errs.length > 0) throw new Error('invalid input:\n- ' + errs.join('\n- '))
  const text: string = await $.fs.read(prev.specPath)
  const section = String(args.spec_section ?? '')
  const q = locate(text, String(args.spec_quote ?? ''), section, prev.specPath, 'focus', errs)
  const shown = q.quoteStart >= 0 ? sectionOf(text, q) : section ? (spec.sectionByTitle(text, section)?.start ?? -1) : -1

  const code = resolved
    ? await Promise.all(
        resolved.map(r =>
          codeView($, r.path, r.start_line, r.end_line, `${rel(r.path)}${r.symbol ? ' ' + r.symbol : `:${r.start_line}-${r.end_line}`}`),
        ),
      )
    : await resolveCode($, 'focus', (args.code_refs as check.CodeRefArg[] | undefined) ?? [], errs)

  // In pane mode a bad ref fails the call, as the bridge's does.
  if (via === 'pane' && errs.length > 0) fail(errs)

  const { how, ...quote } = q
  await update($, viewAtom, () => ({ ...prev, ...quote, text, match: how, code, via, error: errs.join('; ') }))
  await update($, uiAtom, () => ({ shown, codeTab: 0 }))
  void $.ui.open({ id: PANE, title: 'Spec' })
  if (q.quoteStart >= 0) scrollToQuote($)
  return { ok: true, spec_match: how || undefined, docs: [], code: code.map(c => ({ path: c.path, start_line: c.startLine })) }
}

// ---------------------------------------------------------------- questions

async function ask($: EngineInterface, args: Args, signal: AbortSignal): Promise<AskOutput> {
  const invalid = check.ask(args)
  if (invalid.length > 0) throw new Error('invalid input:\n- ' + invalid.join('\n- '))
  const prev = await read($, viewAtom)
  if (!prev) throw new Error('ask: no spec is shown yet; call spec_show first')
  const text: string = await $.fs.read(prev.specPath)

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
      allowOther: !!q.allow_other,
      code: await resolveCode($, label, q.code_refs ?? [], errs),
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
    drafts: {},
    error: '',
    result: null,
  }))
  // Asking again reopens a finished spec.
  await update($, viewAtom, v => (v ? { ...v, text, isDone: false } : v))
  await update($, uiAtom, () => ({ shown: first ? sectionOf(text, first) : -1, codeTab: 0 }))
  void $.ui.open({ id: PANE, title: 'Spec' })
  $.ui.toast(`Claude asks ${questions.length === 1 ? 'a question' : `${questions.length} questions`} in the ai-specview pane`)
  if (first && first.quoteStart >= 0) scrollToQuote($)
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
  await update($, viewAtom, v => (v ? { ...v, ...noQuote, match: '', code: [], isDone: true, summary } : v))
  await update($, uiAtom, () => ({ shown: -1, codeTab: 0 }))
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
    const other = q.allowOther && d.other.trim() ? d.other.trim() : null
    if (selected.length === 0 && other === null) missing.push(q.id)
    answers.push({ id: q.id, selected, other, notes })
  }
  return { answers, deferred, missing }
}

const isAnswered = (q: QuestionView, d: Draft | undefined) =>
  !!d && (d.later || d.selected.length > 0 || (q.allowOther && d.other.trim() !== ''))

async function gotoQuestion($: EngineInterface, k: number) {
  const a = await read($, askAtom)
  const q = a?.questions[k]
  if (!a || !q) return
  await update($, askAtom, cur => (cur ? { ...cur, index: k } : cur))
  const v = await read($, viewAtom)
  await update($, uiAtom, () => ({ shown: v ? sectionOf(v.text, q) : -1, codeTab: 0 }))
  if (q.quoteStart >= 0) scrollToQuote($)
}

async function editDraft($: EngineInterface, id: string, fn: (d: Draft) => Draft) {
  await update($, askAtom, cur => (cur ? { ...cur, error: '', drafts: { ...cur.drafts, [id]: fn(cur.drafts[id] ?? emptyDraft()) } } : cur))
}

async function pickOption($: EngineInterface, q: QuestionView, label: string) {
  await editDraft($, q.id, d => {
    if (!q.multi) return { ...d, selected: [label], later: false }
    const has = d.selected.includes(label)
    return { ...d, selected: has ? d.selected.filter(s => s !== label) : [...d.selected, label], later: false }
  })
  // A single choice moves on to the next unanswered question.
  const a = await read($, askAtom)
  if (!a || q.multi) return
  const next = a.questions.findIndex((other, k) => k > a.index && !isAnswered(other, a.drafts[other.id]))
  if (next >= 0) await gotoQuestion($, next)
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

export const register: Register = (on, options) => {
  ctx.configured = MODES.includes(options.mode as Mode) ? (options.mode as Mode) : 'both'

  on('session.start', async ($, e, next) => {
    ctx.cwd = e.cwd
    await $.command.register({
      name: 'specview-mode',
      description: 'Show or set where ai-specview draws: both, pane or browser',
    })
    await $.command.register({ name: 'specview-pane', description: 'Open the ai-specview pane' })
    if (await read($, viewAtom)) void $.ui.open({ id: PANE, title: 'Spec' })
    return next(e)
  })

  on('command.run', { command: 'specview-mode' }, async ($, e) => {
    const want = e.args.trim() as Mode
    if (!want) return { text: `ai-specview mode: ${await modeOf($)} (configured default: ${ctx.configured}).` }
    if (!MODES.includes(want)) return { text: `Unknown mode "${want}". Use both, pane or browser.` }
    await update($, modeAtom, () => want)
    if (want === 'browser') await $.ui.close({ id: PANE }).catch(() => undefined)
    return { text: `ai-specview mode set to ${want} for this session.` }
  })

  on('command.run', { command: 'specview-pane' }, async $ => {
    await $.ui.open({ id: PANE, title: 'Spec' })
    return { text: 'ai-specview pane opened.' }
  })

  on('tool.call', async ($, e, next) => {
    // In pane mode no daemon answers ai-specview's hook, so the mod blocks the
    // built-in question tool itself while a spec is open.
    if (e.tool === 'AskUserQuestion') {
      if ((await modeOf($)) !== 'pane') return next(e)
      const v = await read($, viewAtom)
      return v && !v.isDone ? { deny: BLOCK_REASON } : next(e)
    }

    const hit = TOOL.exec(e.tool)
    if (!hit) return next(e)
    const mode = await modeOf($)
    if (mode === 'browser') return next(e)
    const name = hit[1]
    const args = e as unknown as Args

    if (mode === 'both') {
      const ran = await next(e)
      if (ran.deny !== undefined || ran.isError) return ran
      try {
        if (name === 'spec_show') {
          await showSpec($, args, 'bridge')
        } else if (name === 'focus') {
          let resolved: ResolvedRef[] | null = null
          try {
            resolved = (JSON.parse(ran.text ?? '{}').code as ResolvedRef[] | undefined) ?? null
          } catch {
            resolved = null
          }
          await focus($, args, 'bridge', resolved)
        } else if (name === 'spec_done') {
          await specDone($, args)
        }
      } catch (err) {
        $.ui.status(`ai-specview pane: ${(err as Error).message.slice(0, 80)}`)
      }
      return ran
    }

    // Pane mode: the mod serves every tool. An MCP tool's result is text, so
    // each answer is the JSON the bridge would send.
    try {
      let out: unknown
      if (name === 'spec_show') {
        const { file, text } = await showSpec($, args, 'pane')
        out = { ok: true, primary: rel(file), sections: spec.headings(text).map(h => h.title), artifacts: [] }
      } else if (name === 'focus') {
        out = await focus($, args, 'pane', null)
      } else if (name === 'ask') {
        out = await ask($, args, next.signal)
      } else if (name === 'ask_wait') {
        out = await askWait($, args, next.signal)
      } else {
        out = await specDone($, args)
      }
      return { result: JSON.stringify(out) }
    } catch (err) {
      return { deny: (err as Error).message }
    }
  }).catch(async ($, e, next) => {
    if (next.called) return next(e)
    // In pane mode the bridge must not get a call the pane serves: it has no spec.
    if (TOOL.test(e.tool) && (await modeOf($)) === 'pane') {
      return { deny: 'ai-specview pane: the call was interrupted or failed. Call it again.' }
    }
    $.ui.status('ai-specview pane: a hook failed; the call went on without it')
    return next(e)
  })

  on('ui.render', { component: 'Pane', requestId: PANE }, async ($, e) => {
    const { Box, Text, Markdown, Code, Select, Button, Input } = $.ui.resolve(e)
    const view = await read($, viewAtom)
    const ui = await read($, uiAtom)
    const a = await read($, askAtom)
    const mode = await modeOf($)
    if (!view) return <Text dimColor>No spec shown yet. Mode: {mode}.</Text>

    const text = view.text
    const asking = a && !a.result && mode === 'pane' ? a : null
    const question = asking ? asking.questions[asking.index] : undefined
    // What the spec and code columns show: the current question's refs while
    // questions wait, else the last focus.
    const at: QuoteView = question ?? view
    const codes = question ? question.code : view.code

    const heads = spec.headings(text)
    const section = ui.shown >= 0 ? spec.sectionAt(text, ui.shown) : null
    const lo = section?.start ?? 0
    const hi = section?.end ?? text.length
    const shownText = text.slice(lo, hi)
    const hasQuote = at.quoteStart >= lo && at.quoteEnd <= hi && at.quoteStart >= 0
    const parts = hasQuote
      ? spec.splitAround(shownText, at.quoteStart - lo, at.quoteEnd - lo)
      : { before: shownText, focus: '', after: '' }
    const wide = e.props.placement === 'dock' && e.props.bodyColumns >= WIDE
    const tab = Math.min(ui.codeTab, Math.max(0, codes.length - 1))
    const active = codes[tab]
    const focusAt = sectionOf(text, at)
    const showSection = (where: number) => update($, uiAtom, u => ({ ...u, shown: where }))

    const options = [
      { value: '-1', label: 'Whole spec' },
      ...heads.map(h => ({
        value: String(h.offset),
        label: `${'  '.repeat(h.level - 1)}${h.title}${view.changedSections.includes(h.title) ? '  (changed)' : ''}`,
      })),
    ]

    const specColumn = (
      <Box key="spec-col" flexDirection="column" width={wide ? '55%' : '100%'} flexShrink={1}>
        {parts.before !== '' && <Markdown key="before" text={parts.before.slice(-MAX_MARKDOWN)} />}
        {parts.focus !== '' && (
          <Box key="quote" flexDirection="column" borderStyle="round" borderColor="yellow" paddingX={1}>
            <Text color="yellow" size="caption">
              {question ? `Question ${asking!.index + 1}` : 'Focus'}
              {at.quoteSection && section?.title !== at.quoteSection ? ` in ${at.quoteSection}` : ''}
            </Text>
            <Markdown key="quote-text" text={parts.focus} />
          </Box>
        )}
        {parts.after !== '' && <Markdown key="after" text={parts.after.slice(0, MAX_MARKDOWN)} />}
      </Box>
    )

    const codeBlock =
      codes.length === 0 ? null : (
        <Box key="code" flexDirection="column" rowGap={1}>
          <Box flexDirection="row" flexWrap="wrap" columnGap={1}>
            {codes.map((c, i) => (
              <Button
                key={`code-${i}`}
                variant={i === tab ? 'primary' : 'secondary'}
                onPress={() => update($, uiAtom, u => ({ ...u, codeTab: i }))}
              >
                {c.label.replace(/^.*\//, '')}
              </Button>
            ))}
          </Box>
          {active && (
            <Box flexDirection="column">
              <Text dimColor>{active.label}</Text>
              <Code source={active.source.slice(0, MAX_CODE)} path={active.path} startLine={active.startLine} />
            </Box>
          )}
        </Box>
      )

    let askPanel = null
    if (asking && question) {
      const n = asking.questions.length
      const d = asking.drafts[question.id] ?? emptyDraft()
      askPanel = (
        <Box key="ask" flexDirection="column" borderStyle="round" borderColor="cyan" paddingX={1} rowGap={1}>
          <Box flexDirection="row" flexWrap="wrap" columnGap={1} alignItems="center">
            <Text bold>{asking.title || 'Questions'}</Text>
            {asking.questions.map((q, k) => (
              <Button
                key={`step-${k}`}
                plain
                variant={k === asking.index ? 'primary' : 'secondary'}
                onPress={() => gotoQuestion($, k)}
              >
                {`${k + 1}${isAnswered(q, asking.drafts[q.id]) ? ' ✓' : ''}`}
              </Button>
            ))}
          </Box>
          <Markdown key="q-text" text={question.text} />
          <Box flexDirection="column">
            {question.options.map((o, k) => {
              const picked = !d.later && d.selected.includes(o.label)
              return (
                <Button
                  key={`opt-${k}`}
                  variant={picked ? 'primary' : 'secondary'}
                  onPress={() => pickOption($, question, o.label)}
                >
                  {`${question.multi ? (picked ? '☑' : '☐') : picked ? '●' : '○'} ${o.label}${o.recommended ? '  (recommended)' : ''}`}
                  {o.description !== '' && <Text dimColor>{o.description}</Text>}
                </Button>
              )
            })}
            <Button
              key="later"
              variant={d.later ? 'primary' : 'secondary'}
              onPress={() =>
                editDraft($, question.id, cur =>
                  cur.later ? { ...cur, later: false } : { ...cur, later: true, selected: [], other: '' },
                )
              }
            >
              {`${d.later ? '●' : '○'} Decide later`}
            </Button>
          </Box>
          {question.allowOther && (
            <Input
              key={`other-${question.id}`}
              label="Other"
              placeholder="Your own answer"
              value={d.other}
              onInput={value => void editDraft($, question.id, cur => ({ ...cur, other: value, later: value.trim() ? false : cur.later }))}
              onSubmit={value => void editDraft($, question.id, cur => ({ ...cur, other: value }))}
            />
          )}
          <Input
            key={`notes-${question.id}`}
            label="Notes"
            placeholder="Optional, sent with the answer"
            value={d.notes}
            onInput={value => void editDraft($, question.id, cur => ({ ...cur, notes: value }))}
            onSubmit={value => void editDraft($, question.id, cur => ({ ...cur, notes: value }))}
          />
          {asking.error !== '' && <Text color="red">{asking.error}</Text>}
          <Box flexDirection="row" flexWrap="wrap" columnGap={1}>
            {asking.index > 0 && (
              <Button key="prev" onPress={() => gotoQuestion($, asking.index - 1)}>
                Previous
              </Button>
            )}
            {asking.index < n - 1 && (
              <Button key="next" onPress={() => gotoQuestion($, asking.index + 1)}>
                Next
              </Button>
            )}
            <Button key="send" variant="primary" onPress={() => sendAnswers($)}>
              Send answers
            </Button>
            <Button key="in-chat" onPress={press => backToChat($, press.surface)}>
              Back to chat
            </Button>
            <Button key="skip" onPress={() => skip($)}>
              Skip
            </Button>
          </Box>
        </Box>
      )
    }

    const side =
      askPanel || codeBlock ? (
        <Box key="side-col" flexDirection="column" width={wide ? '45%' : '100%'} flexShrink={1} rowGap={1}>
          {askPanel}
          {codeBlock}
        </Box>
      ) : null

    return (
      <Box flexDirection="column" rowGap={1}>
        <Box flexDirection="column">
          <Text bold>{view.title}</Text>
          <Text dimColor>
            mode {mode}, last update from the {view.via}
            {view.match && !question ? `, quote match ${view.match}` : ''}
          </Text>
          {view.error !== '' && <Text color="red">{view.error}</Text>}
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
          <Select
            key="outline"
            label="Section"
            options={options}
            value={String(ui.shown)}
            onSelect={v => void showSection(Number(v))}
          />
          {view.changedSections.length > 0 && <Text dimColor>Changed:</Text>}
          {view.changedSections.map((title, i) => (
            <Button key={`changed-${i}`} plain onPress={() => showSection(spec.sectionByTitle(text, title)?.start ?? -1)}>
              {title}
            </Button>
          ))}
          {at.quoteStart >= 0 && ui.shown !== focusAt && ui.shown !== -1 && (
            <Button key="to-quote" plain onPress={() => showSection(focusAt)}>
              {question ? 'Back to the question' : 'Back to the focus'}
            </Button>
          )}
        </Box>
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
