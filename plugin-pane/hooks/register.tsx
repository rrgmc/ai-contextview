// ai-specview-pane: draws the ai-specview spec and code in a pane beside the chat.
//
// It hooks the ai-specview MCP tools by name, so the model, the skill and the
// browser viewer stay as they are. The mode picks who serves spec_show and
// focus:
//
//   browser  the MCP bridge alone (the mod does nothing)
//   both     the bridge runs, then the pane mirrors the call, using the
//            bridge's resolved code ranges
//   pane     the mod answers spec_show and focus itself; no daemon starts for
//            them. ask, ask_wait and spec_done still go to the bridge, which is
//            first sent the last spec_show so it knows the spec.

import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register } from 'claude-code'

import type { CodeView, Mode, PaneUi, SpecView } from '../types'
import * as spec from './spec'

const PANE = 'ai-specview'
const MODES: readonly Mode[] = ['both', 'pane', 'browser']
const TOOL = /^mcp__.*ai-specview__(spec_show|focus|ask|ask_wait|spec_done)$/

// Change a tag when the stored shape changes: it declines an older value.
const viewAtom = atom({ plugin: 'ai-specview-pane', key: 'view' } as const, null as SpecView | null, {
  shape: 'view-2',
})
const uiAtom = atom({ plugin: 'ai-specview-pane', key: 'ui' } as const, { shown: -1, codeTab: 0 } as PaneUi, {
  shape: 'ui-1',
})
const modeAtom = atom({ plugin: 'ai-specview-pane', key: 'mode' } as const, null as Mode | null, {
  shape: 'mode-1',
})

type Args = Record<string, unknown>
type CodeRefArg = { path?: string; symbol?: string; lines?: string }
type ResolvedRef = { path: string; start_line: number; end_line: number; symbol?: string }

const MAX_MARKDOWN = 50_000
const MAX_CODE = 40_000
// The pane splits into spec and code columns from this many cells across.
const WIDE = 100

type Ctx = {
  configured: Mode
  cwd: string
  // The last spec_show the pane served, kept to brief the bridge before an ask.
  lastSpecShow: { tool: string; args: Args } | null
  bridgeKnowsSpec: boolean
  passThrough: boolean
}

// Module state: it starts over on a reload, and session.start sets cwd again.
const ctx: Ctx = { configured: 'both', cwd: '', lastSpecShow: null, bridgeKnowsSpec: false, passThrough: false }

const abs = (p: string) => (p.startsWith('/') ? p : `${ctx.cwd.replace(/\/$/, '')}/${p}`)
const rel = (p: string) => (ctx.cwd && p.startsWith(ctx.cwd + '/') ? p.slice(ctx.cwd.length + 1) : p)

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
    specPath: file,
    title: rel(file),
    text,
    changedSections: (args.changed_sections as string[] | undefined) ?? [],
    quote: '',
    quoteStart: -1,
    quoteEnd: -1,
    quoteSection: '',
    match: '',
    code: [],
    via,
    error: '',
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
async function resolveCode($: EngineInterface, refs: CodeRefArg[], errs: string[]): Promise<CodeView[]> {
  const out: CodeView[] = []
  for (const r of refs) {
    if (!r.path) {
      errs.push('code ref without a path')
      continue
    }
    const p = abs(r.path)
    if (!(await $.fs.exists(p))) {
      errs.push(`code ref ${r.path} does not exist`)
      continue
    }
    if (r.lines) {
      const range = spec.parseLines(r.lines)
      if (!range) {
        errs.push(`code ref ${r.path}: lines ${r.lines} is not N or N-M`)
        continue
      }
      out.push(await codeView($, p, range.start, range.end, `${rel(p)}:${r.lines}`))
    } else if (r.symbol) {
      const range = spec.findGoSymbol(await $.fs.read(p), r.symbol)
      if (!range) {
        errs.push(`code ref ${r.path}: symbol ${r.symbol} not found`)
        continue
      }
      out.push(await codeView($, p, range.start, range.end, `${rel(p)} ${r.symbol}`))
    } else {
      errs.push(`code ref ${r.path} needs symbol or lines`)
    }
  }
  return out
}

async function focus($: EngineInterface, args: Args, via: SpecView['via'], resolved: ResolvedRef[] | null) {
  const prev = await read($, viewAtom)
  if (!prev) throw new Error('focus: no spec is shown yet; call spec_show first')
  const text: string = await $.fs.read(prev.specPath)
  const quote = String(args.spec_quote ?? '')
  const section = String(args.spec_section ?? '')
  const errs: string[] = []

  let shown = -1
  let match = ''
  let quoteSection = section
  let quoteStart = -1
  let quoteEnd = -1
  if (quote) {
    const m = spec.findQuote(text, quote)
    if (m) {
      const s = spec.sectionAt(text, m.start)
      shown = s.title ? s.start : -1
      quoteSection = s.title || section
      quoteStart = m.start
      quoteEnd = m.end
      match = m.how
    } else {
      match = 'none'
      errs.push(`quote ${JSON.stringify(quote)} not found in ${rel(prev.specPath)} (copy it verbatim from the file)`)
    }
  } else if (section) {
    shown = spec.sectionByTitle(text, section)?.start ?? -1
  }

  const code = resolved
    ? await Promise.all(
        resolved.map(r =>
          codeView($, r.path, r.start_line, r.end_line, `${rel(r.path)}${r.symbol ? ' ' + r.symbol : `:${r.start_line}-${r.end_line}`}`),
        ),
      )
    : await resolveCode($, (args.code_refs as CodeRefArg[] | undefined) ?? [], errs)

  // In pane mode a bad ref fails the call, as the bridge's does.
  if (via === 'pane' && errs.length > 0) throw new Error('fix these and call again:\n- ' + errs.join('\n- '))

  await update($, viewAtom, () => ({
    ...prev,
    text,
    quote,
    quoteStart,
    quoteEnd,
    quoteSection,
    match,
    code,
    via,
    error: errs.join('; '),
  }))
  await update($, uiAtom, () => ({ shown, codeTab: 0 }))
  void $.ui.open({ id: PANE, title: 'Spec' })
  // Bring the focused block into view once the pane has drawn it.
  if (quoteStart >= 0) $.clock.after(250, () => void $.ui.scroll({ to: { key: 'quote' }, in: PANE, block: 'center' }).catch(() => undefined))
  return { ok: true, spec_match: match || undefined, docs: [], code: code.map(c => ({ path: c.path, start_line: c.startLine })) }
}

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
    const hit = TOOL.exec(e.tool)
    if (!hit || ctx.passThrough) return next(e)
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
          ctx.bridgeKnowsSpec = true
        } else if (name === 'focus') {
          let resolved: ResolvedRef[] | null = null
          try {
            resolved = (JSON.parse(ran.text ?? '{}').code as ResolvedRef[] | undefined) ?? null
          } catch {
            resolved = null
          }
          await focus($, args, 'bridge', resolved)
        }
      } catch (err) {
        $.ui.status(`ai-specview pane: ${(err as Error).message.slice(0, 80)}`)
      }
      return ran
    }

    // Pane mode.
    try {
      if (name === 'spec_show') {
        const { text } = await showSpec($, args, 'pane')
        ctx.lastSpecShow = { tool: e.tool, args: { ...args } }
        ctx.bridgeKnowsSpec = false
        // An MCP tool's result is text (or content blocks): send the JSON as the bridge does.
        return {
          result: JSON.stringify({
            ok: true,
            primary: rel(String((await read($, viewAtom))?.specPath ?? '')),
            sections: spec.headings(text).map(h => h.title),
            artifacts: [],
          }),
        }
      }
      if (name === 'focus') return { result: JSON.stringify(await focus($, args, 'pane', null)) }
    } catch (err) {
      return { deny: (err as Error).message }
    }

    // ask, ask_wait, spec_done: the bridge serves them, and needs the spec first.
    if (!ctx.bridgeKnowsSpec && ctx.lastSpecShow) {
      const { tool: _t, tool_use_id: _id, agentId: _a, ...rest } = ctx.lastSpecShow.args as Args & {
        tool_use_id?: string
        agentId?: string
      }
      ctx.passThrough = true
      try {
        // The connected MCP tools' types make $.tool.call's overloads too deep for
        // tsc (TS2589), and the engine requires the call spelled out here.
        // @ts-ignore TS2589
        await $.tool.call({ tool: ctx.lastSpecShow.tool, ...rest })
        ctx.bridgeKnowsSpec = true
      } finally {
        ctx.passThrough = false
      }
    }
    return next(e)
  }).catch(($, e, next) => {
    // A failure of the mirror must never block the tool: pass the call on.
    $.ui.status('ai-specview pane: a hook failed; the call went to the bridge')
    return next(e)
  })

  on('ui.render', { component: 'Pane', requestId: PANE }, async ($, e) => {
    const { Box, Text, Markdown, Code, Select, Button } = $.ui.resolve(e)
    const view = await read($, viewAtom)
    const ui = await read($, uiAtom)
    const mode = await modeOf($)
    if (!view) return <Text dimColor>No spec shown yet. Mode: {mode}.</Text>

    const text = view.text
    const heads = spec.headings(text)
    const section = ui.shown >= 0 ? spec.sectionAt(text, ui.shown) : null
    const lo = section?.start ?? 0
    const hi = section?.end ?? text.length
    const shownText = text.slice(lo, hi)
    const hasQuote = view.quoteStart >= lo && view.quoteEnd <= hi && view.quoteStart >= 0
    const parts = hasQuote
      ? spec.splitAround(shownText, view.quoteStart - lo, view.quoteEnd - lo)
      : { before: shownText, focus: '', after: '' }
    const wide = e.props.placement === 'dock' && e.props.bodyColumns >= WIDE
    const tab = Math.min(ui.codeTab, Math.max(0, view.code.length - 1))
    const active = view.code[tab]
    const showSection = (at: number) => update($, uiAtom, u => ({ ...u, shown: at }))
    const focusAt = view.quoteStart >= 0 ? spec.sectionAt(text, view.quoteStart).start : -1

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
              Focus{view.quoteSection ? ` in ${view.quoteSection}` : ''}
            </Text>
            <Markdown key="quote-text" text={parts.focus} />
          </Box>
        )}
        {parts.after !== '' && <Markdown key="after" text={parts.after.slice(0, MAX_MARKDOWN)} />}
      </Box>
    )

    const codeColumn =
      view.code.length === 0 ? null : (
        <Box key="code-col" flexDirection="column" width={wide ? '45%' : '100%'} flexShrink={1} rowGap={1}>
          <Box flexDirection="row" flexWrap="wrap" columnGap={1}>
            {view.code.map((c, i) => (
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

    return (
      <Box flexDirection="column" rowGap={1}>
        <Box flexDirection="column">
          <Text bold>{view.title}</Text>
          <Text dimColor>
            mode {mode}, last update from the {view.via}
            {view.match ? `, quote match ${view.match}` : ''}
          </Text>
          {view.error !== '' && <Text color="red">{view.error}</Text>}
        </Box>
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
            <Button
              key={`changed-${i}`}
              plain
              onPress={() => showSection(spec.sectionByTitle(text, title)?.start ?? -1)}
            >
              {title}
            </Button>
          ))}
          {view.quoteStart >= 0 && ui.shown !== focusAt && ui.shown !== -1 && (
            <Button key="to-quote" plain onPress={() => showSection(focusAt)}>
              Back to the focus
            </Button>
          )}
        </Box>
        <Box flexDirection={wide ? 'row' : 'column'} columnGap={2} rowGap={1}>
          {specColumn}
          {codeColumn}
        </Box>
      </Box>
    )
  })
}
