import { expect, mock, test } from 'claude-code/testing'

import * as spec from '../hooks/spec'

const SPEC_SHOW = 'mcp__plugin_ai-specview_ai-specview__spec_show'
const FOCUS = 'mcp__plugin_ai-specview_ai-specview__focus'
const ASK = 'mcp__plugin_ai-specview_ai-specview__ask'

const SPEC = `# Widget

Intro text.

## Storage

Widgets are kept in memory and lost on restart.

Another paragraph.

## API

One endpoint.
`

const GO = `package store

type Store struct {
	items map[string]string
}

// Get returns the item under k.
func (s *Store) Get(k string) string {
	return s.items[k]
}
`

const FILES: Record<string, string> = {
  '/repo/docs/spec.md': SPEC,
  '/repo/store/store.go': GO,
}

// The pane as the desktop docks it beside the chat; `columns` sets its width.
function pane(surface: 'terminal' | 'desktop', columns: number) {
  return {
    plugin: 'ai-specview-pane',
    surface,
    component: 'Pane',
    requestId: 'ai-specview',
    props: {
      title: 'Spec',
      isFocused: false,
      bodyColumns: columns,
      placement: 'dock',
      scroll: { offset: 0, bodyRows: 40 },
      view: {},
    },
  } as const
}

// $.tool.call typed loosely: the connected MCP tools' types are too deep to check here.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const call = ($: any, input: Record<string, unknown>) => $.tool.call(input)

// What the last $.ui.copy put on the clipboard.
const clipboard = { text: '' }

// The engine beneath the mod: a session in /repo with two files.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function engine(on: any, bridge: (e: any) => unknown) {
  // A clock that moves only when the test moves it. ask pauses with `sleep 1`
  // through $.process.run: here each pause is a second on this clock.
  const clk = mock.clock(on)
  on('process.run', async () => {
    await clk.sleep(1000)
    return { value: { exitCode: 0, stdout: '', stderr: '' } }
  })
  on('command.register', () => ({ value: { command: 'x' } }))
  on('ui.open', () => ({ value: { isPlaced: true } }))
  on('ui.scroll', () => ({ value: {} }))
  on('ui.toast', () => ({ value: undefined }))
  on('ui.status', () => ({ value: undefined }))
  on('ui.copy', (_: unknown, e: { text: string }) => {
    clipboard.text = e.text
    return { value: { isCopied: true } }
  })
  on('session.start', (_: unknown, e: { cwd: string }) => ({ cwd: e.cwd }))
  on('fs.read', (_: unknown, e: { path: string }) => ({ value: FILES[e.path] }))
  on('fs.exists', (_: unknown, e: { path: string }) => ({ value: e.path in FILES }))
  on('fs.stat', () => ({ value: { kind: 'file', size: 1, mtimeMs: 0, isLink: false } }))
  on('tool.call', (_: unknown, e: unknown) => bridge(e))
  return clk
}

test('splitAround cuts at blank lines, never inside a code fence', () => {
  const md = 'a\n\n```\nx\n\ny\n```\n\nb\n'
  const at = md.indexOf('y')
  const parts = spec.splitAround(md, at, at + 1)
  expect(parts.before).toBe('a\n\n')
  expect(parts.focus.startsWith('```')).toBe(true)
  expect(parts.after).toBe('b\n')
})

test('findGoSymbol takes the doc comment', () => {
  expect(spec.findGoSymbol(GO, 'store.(*Store).Get')).toEqual({ start: 7, end: 10 })
})

test('pane mode: spec and code side by side when wide, stacked when narrow', { options: { mode: 'pane' } }, async ($, on) => {
  const seen: string[] = []
  engine(on, (e: { tool: string }) => {
    seen.push(e.tool)
    return { result: {} }
  })
  await $.session.start({ cwd: '/repo', surface: 'desktop', isInteractive: true })

  const shown = await call($, { tool: SPEC_SHOW, path: 'docs/spec.md' })
  expect(JSON.parse(shown.result as string).sections).toEqual(['Widget', 'Storage', 'API'])
  await call($, {
    tool: FOCUS,
    spec_quote: 'kept in memory',
    code_refs: [{ path: 'store/store.go', symbol: 'store.(*Store).Get' }],
  })
  expect(seen).toEqual([])

  for (const surface of ['terminal', 'desktop'] as const) {
    const wide = await $.ui.mount(pane(surface, 140))
    expect((await wide.find({ key: 'spec-col' }))?.props.width).toBe('55%')
    expect((await wide.find({ key: 'side-col' }))?.props.width).toBe('45%')
    // The focus box holds the quote's paragraph alone, the quote in bold.
    const box = await wide.find({ key: 'quote-text' })
    expect(box?.props.text).toBe('Widgets are **kept in memory** and lost on restart.\n\n')
    // Only the Storage section shows, so API's text is not drawn.
    expect((await wide.find({ key: 'after' }))?.props.text).toBe('Another paragraph.\n\n')
    expect((await wide.find({ type: 'Code' }))?.props.startLine).toBe(7)
    await wide.unmount()

    const narrow = await $.ui.mount(pane(surface, 60))
    expect((await narrow.find({ key: 'spec-col' }))?.props.width).toBe('100%')
    await narrow.unmount()
  }
})

test('the section picker shows the whole spec or one section', { options: { mode: 'pane' } }, async ($, on) => {
  engine(on, () => ({ result: {} }))
  await $.session.start({ cwd: '/repo', surface: 'desktop', isInteractive: true })
  await call($, { tool: SPEC_SHOW, path: 'docs/spec.md' })
  await call($, { tool: FOCUS, spec_quote: 'kept in memory' })

  const ui = await $.ui.mount(pane('desktop', 140))
  await ui.select({ key: 'outline', value: '-1' })
  expect(String((await ui.find({ key: 'before' }))?.props.text)).toContain('# Widget')
  expect(String((await ui.find({ key: 'after' }))?.props.text)).toContain('One endpoint.')

  const api = String(SPEC.indexOf('## API'))
  await ui.select({ key: 'outline', value: api })
  expect((await ui.find({ key: 'before' }))?.props.text).toBe('## API\n\nOne endpoint.\n')
  expect(await ui.find({ key: 'quote' })).toBeUndefined()

  await ui.press({ key: 'to-quote' })
  expect(await ui.find({ key: 'quote' })).toBeDefined()
  await ui.unmount()
})

test('code tabs switch the code shown', { options: { mode: 'pane' } }, async ($, on) => {
  engine(on, () => ({ result: {} }))
  await $.session.start({ cwd: '/repo', surface: 'desktop', isInteractive: true })
  await call($, { tool: SPEC_SHOW, path: 'docs/spec.md' })
  await call($, {
    tool: FOCUS,
    spec_quote: 'One endpoint.',
    code_refs: [
      { path: 'store/store.go', lines: '3-5' },
      { path: 'store/store.go', symbol: 'store.(*Store).Get' },
    ],
  })

  const ui = await $.ui.mount(pane('desktop', 140))
  expect((await ui.find({ type: 'Code' }))?.props.startLine).toBe(3)
  await ui.press({ key: 'code-1' })
  expect((await ui.find({ type: 'Code' }))?.props.startLine).toBe(7)
  expect((await ui.find({ key: 'code-1' }))?.props.variant).toBe('primary')
  await ui.unmount()
})

test('pane mode fails focus on a quote that is not in the spec', { options: { mode: 'pane' } }, async ($, on) => {
  engine(on, () => ({ result: {} }))
  await $.session.start({ cwd: '/repo', surface: 'desktop', isInteractive: true })
  await call($, { tool: SPEC_SHOW, path: 'docs/spec.md' })

  const ran = await call($, { tool: FOCUS, spec_quote: 'kept on disk' })
  expect(ran.isError || ran.deny !== undefined).toBe(true)
})

test('both mode runs the bridge and draws its resolved code range', { options: { mode: 'both' } }, async ($, on) => {
  const seen: string[] = []
  engine(on, (e: { tool: string }) => {
    seen.push(e.tool)
    if (e.tool === FOCUS) {
      const code = [{ path: '/repo/store/store.go', start_line: 3, end_line: 5, source: 'lines' }]
      return { result: JSON.stringify({ ok: true, code }), text: JSON.stringify({ ok: true, code }) }
    }
    return { result: '{"ok":true}', text: '{"ok":true}' }
  })
  await $.session.start({ cwd: '/repo', surface: 'desktop', isInteractive: true })
  await call($, { tool: SPEC_SHOW, path: 'docs/spec.md' })
  await call($, { tool: FOCUS, spec_section: 'API', code_refs: [{ path: 'store/store.go', lines: '3-5' }] })
  expect(seen).toEqual([SPEC_SHOW, FOCUS])

  const ui = await $.ui.mount(pane('desktop', 140))
  expect((await ui.find({ type: 'Text', text: /store\.go:3-5/ }))?.text).toBe('store/store.go:3-5')
  expect(await ui.find({ type: 'Text', text: /from the bridge/ })).toBeDefined()
  await ui.unmount()
})

test('browser mode leaves the pane empty', { options: { mode: 'browser' } }, async ($, on) => {
  engine(on, () => ({ result: '{}', text: '{}' }))
  await $.session.start({ cwd: '/repo', surface: 'desktop', isInteractive: true })
  await call($, { tool: SPEC_SHOW, path: 'docs/spec.md' })

  const ui = await $.ui.mount(pane('desktop', 140))
  expect(await ui.find({ type: 'Text', text: /No spec shown yet/ })).toBeDefined()
  await ui.unmount()
})

const QUESTIONS = [
  {
    id: 'q1',
    text: 'Where are widgets kept?',
    options: [{ label: 'Memory' }, { label: 'Disk (Recommended)', description: 'Survives a restart' }],
    spec_quote: 'kept in memory',
    allow_other: true,
  },
  {
    id: 'q2',
    text: 'How many endpoints?',
    options: [{ label: 'One' }, { label: 'Two' }],
    spec_quote: 'One endpoint.',
    code_refs: [{ path: 'store/store.go', symbol: 'store.(*Store).Get' }],
  },
]

// Starts an ask without waiting for it, then mounts the pane once the form shows.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
async function startAsk($: any, clk: { settle: () => Promise<void> }) {
  await $.session.start({ cwd: '/repo', surface: 'desktop', isInteractive: true })
  await call($, { tool: SPEC_SHOW, path: 'docs/spec.md' })
  const pending = call($, { tool: ASK, title: 'Storage', questions: QUESTIONS })
  await clk.settle()
  const ui = await $.ui.mount(pane('desktop', 140))
  return { pending, ui }
}

// ask polls every second: moving the clock past a poll lets it see the answer.
const POLL = 1100

const result = async (p: Promise<{ result?: unknown }>) => JSON.parse((await p).result as string)

test('pane mode: questions are answered in the pane', { options: { mode: 'pane' } }, async ($, on) => {
  const seen: string[] = []
  const clk = engine(on, (e: { tool: string }) => {
    seen.push(e.tool)
    return { result: '{}' }
  })
  const { pending, ui } = await startAsk($, clk)
  // The first question's quote is framed, and the recommended option is marked.
  expect((await ui.find({ key: 'quote-text' }))?.props.text).toContain('**kept in memory**')
  expect(JSON.stringify((await ui.find({ key: 'opt-1' }))?.props)).toContain('(recommended)')

  // A single choice moves on to q2, whose code shows.
  await ui.press({ key: 'opt-1' })
  expect((await ui.find({ key: 'quote-text' }))?.props.text).toContain('**One endpoint.**')
  expect((await ui.find({ type: 'Code' }))?.props.startLine).toBe(7)
  await ui.input({ key: 'notes-q2', text: 'keep it small', kind: 'change' })
  await ui.press({ key: 'opt-0' })
  await ui.press({ key: 'send' })
  await clk.advance(POLL)

  expect(await result(pending)).toEqual({
    status: 'answered',
    answers: [
      { id: 'q1', selected: ['Disk'], other: null, notes: null },
      { id: 'q2', selected: ['One'], other: null, notes: 'keep it small' },
    ],
  })
  expect(seen).toEqual([])
  expect(await ui.find({ key: 'ask' })).toBeUndefined()
  await ui.unmount()
})

test('Send asks for every answer; Decide later defers one', { options: { mode: 'pane' } }, async ($, on) => {
  const clk = engine(on, () => ({ result: '{}' }))
  const { pending, ui } = await startAsk($, clk)
  await ui.press({ key: 'send' })
  expect((await ui.find({ type: 'Text', text: /Answer q1, q2/ }))?.text).toBe('Answer q1, q2, or choose Decide later.')

  await ui.input({ key: 'other-q1', text: 'Both', kind: 'change' })
  await ui.press({ key: 'step-1' })
  await ui.press({ key: 'later' })
  await ui.press({ key: 'send' })
  await clk.advance(POLL)
  const out = await result(pending)
  expect(out.status).toBe('answered')
  expect(out.deferred).toEqual(['q2'])
  expect(out.answers[0]).toEqual({ id: 'q1', selected: [], other: 'Both', notes: null })
  expect(out.message).toContain('decide some questions later')
  await ui.unmount()
})

test('Back to chat copies the questions; Skip dismisses', { options: { mode: 'pane' } }, async ($, on) => {
  const clk = engine(on, () => ({ result: '{}' }))
  const first = await startAsk($, clk)
  await first.ui.press({ key: 'in-chat' })
  await clk.advance(POLL)
  expect((await result(first.pending)).status).toBe('in_chat')
  expect(clipboard.text).toContain('1. Where are widgets kept?')
  expect(clipboard.text).toContain('b) Disk (Survives a restart)')
  await first.ui.unmount()

  const again = call($, { tool: ASK, questions: QUESTIONS })
  await clk.settle()
  const ui = await $.ui.mount(pane('desktop', 140))
  await ui.press({ key: 'skip' })
  await clk.advance(POLL)
  expect((await result(again)).status).toBe('dismissed')
  await ui.unmount()
})

test('ask returns pending after the wait, and ask_wait collects the answer', { options: { mode: 'pane' } }, async ($, on) => {
  const clk = engine(on, () => ({ result: '{}' }))
  const { pending, ui } = await startAsk($, clk)
  await clk.advance(10 * 60_000 + POLL)
  const first = await result(pending)
  expect(first.status).toBe('pending')
  expect(first.question_set_id).toMatch(/^qs_/)

  await ui.press({ key: 'opt-0' })
  await ui.press({ key: 'opt-0' })
  await ui.press({ key: 'send' })
  const out = await result(call($, { tool: 'mcp__plugin_ai-specview_ai-specview__ask_wait', question_set_id: first.question_set_id }))
  expect(out.status).toBe('answered')

  const unknown = await call($, { tool: 'mcp__plugin_ai-specview_ai-specview__ask_wait', question_set_id: 'qs_nope' })
  expect(unknown.isError || unknown.deny !== undefined).toBe(true)
  await ui.unmount()
})

test('ask checks its input and its quotes as the bridge does', { options: { mode: 'pane' } }, async ($, on) => {
  engine(on, () => ({ result: '{}' }))
  await $.session.start({ cwd: '/repo', surface: 'desktop', isInteractive: true })
  await call($, { tool: SPEC_SHOW, path: 'docs/spec.md' })
  const bad = await call($, { tool: ASK, questions: [{ id: 'q1', text: 'x', options: [{ label: 'a' }] }] })
  expect(bad.isError || bad.deny !== undefined).toBe(true)
  const missing = await call($, {
    tool: ASK,
    questions: [{ ...QUESTIONS[0], spec_quote: 'not in the spec' }],
  })
  expect(missing.isError || missing.deny !== undefined).toBe(true)
})

test('pane mode blocks AskUserQuestion until spec_done', { options: { mode: 'pane' } }, async ($, on) => {
  engine(on, () => ({ result: '{}' }))
  await $.session.start({ cwd: '/repo', surface: 'desktop', isInteractive: true })
  await call($, { tool: SPEC_SHOW, path: 'docs/spec.md' })
  const blocked = await call($, { tool: 'AskUserQuestion', questions: [] })
  expect(blocked.isError || blocked.deny !== undefined).toBe(true)

  await call($, { tool: 'mcp__plugin_ai-specview_ai-specview__spec_done', summary: '**Decided**\n\n- memory' })
  const ui = await $.ui.mount(pane('desktop', 140))
  expect(await ui.find({ key: 'done' })).toBeDefined()
  expect((await ui.find({ key: 'summary' }))?.props.text).toContain('memory')
  await ui.unmount()

  const allowed = await call($, { tool: 'AskUserQuestion', questions: [] })
  expect(allowed.isError || allowed.deny !== undefined).toBe(false)
})
