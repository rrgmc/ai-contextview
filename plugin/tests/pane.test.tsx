import { expect, mock, test } from 'claude-code/testing'

import * as spec from '../hooks/spec'

// The mod's own tools, as Claude Code lists them.
const SPEC_SHOW = 'mcp__ai-specview__spec_show'
const FOCUS = 'mcp__ai-specview__focus'
const ASK = 'mcp__ai-specview__ask'
const ASK_WAIT = 'mcp__ai-specview__ask_wait'
const SPEC_DONE = 'mcp__ai-specview__spec_done'

const SPEC = `# Widget

Intro text.

## Storage

Widgets are kept in memory and lost on restart.

Another paragraph.

## API

One endpoint.
`

const DESIGN = `# Design

## Cache

The cache holds at most 100 widgets.
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

// The project in /repo: a single-file spec, a design note beside it, a folder
// spec in docs/change/ and some Go code.
const FILES: Record<string, string> = {
  '/repo/docs/spec.md': SPEC,
  '/repo/docs/design.md': DESIGN,
  '/repo/docs/change/proposal.md': SPEC,
  '/repo/docs/change/notes/cache.md': DESIGN,
  '/repo/docs/change/.hidden.md': 'hidden',
  '/repo/store/store.go': GO,
  // A draft spec in the home folder, outside the project.
  '/home/me/.claude/specs/repo/draft.md': SPEC,
}
const DIRS: Record<string, { name: string; kind: 'file' | 'dir' }[]> = {
  '/repo/docs/change': [
    { name: 'proposal.md', kind: 'file' },
    { name: 'notes', kind: 'dir' },
    { name: '.hidden.md', kind: 'file' },
  ],
  '/repo/docs/change/notes': [{ name: 'cache.md', kind: 'file' }],
}

// The pane as the desktop docks it beside the chat; `columns` sets its width.
function pane(surface: 'terminal' | 'desktop', columns: number) {
  return {
    plugin: 'ai-specview',
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
const result = async (p: Promise<{ result?: unknown }>) => JSON.parse((await p).result as string)
const failed = (r: { isError?: boolean; deny?: string }) => r.isError === true || r.deny !== undefined

// What the last $.ui.copy put on the clipboard.
const clipboard = { text: '' }

// The engine beneath the mod: a session in /repo. Every other tool call gets
// `other`. Returns a clock that moves only when the test moves it.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function engine(on: any, other: (e: { tool: string }) => unknown = () => ({ result: '{}' })) {
  const clk = mock.clock(on)
  mock.env(on, { HOME: '/home/me' })
  // ask pauses with `sleep 1` through $.process.run: here, a second on this clock.
  on('process.run', async () => {
    await clk.sleep(1000)
    return { value: { exitCode: 0, stdout: '', stderr: '' } }
  })
  on('command.register', () => ({ value: { command: 'x' } }))
  on('tool.register', () => ({ value: {} }))
  on('ui.open', () => ({ value: { isPlaced: true } }))
  on('ui.scroll', () => ({ value: {} }))
  on('ui.toast', () => ({ value: undefined }))
  on('ui.status', () => ({ value: undefined }))
  on('ui.copy', (_: unknown, e: { text: string }) => {
    clipboard.text = e.text
    return { value: { isCopied: true } }
  })
  on('session.start', (_: unknown, e: { cwd: string }) => ({ cwd: e.cwd }))
  on('fs.read', (_: unknown, e: { path: string }) => {
    const body = FILES[e.path]
    if (body === undefined) throw new Error(`no such file ${e.path}`)
    return { value: body }
  })
  on('fs.exists', (_: unknown, e: { path: string }) => ({ value: e.path in FILES || e.path in DIRS }))
  on('fs.list', (_: unknown, e: { path: string }) => ({
    value: (DIRS[e.path] ?? []).map(f => ({ ...f, size: 1, mtimeMs: 0 })),
  }))
  on('fs.stat', (_: unknown, e: { path: string }) => ({
    value: { kind: e.path in DIRS ? 'dir' : 'file', size: 1, mtimeMs: 0, isLink: false },
  }))
  on('tool.call', (_: unknown, e: { tool: string }) => other(e))
  return clk
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
async function start($: any, path = 'docs/spec.md') {
  await $.session.start({ cwd: '/repo', surface: 'desktop', isInteractive: true })
  return result(call($, { tool: SPEC_SHOW, path }))
}

// ---------------------------------------------------------------- helpers

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

// ------------------------------------------------------------ spec and focus

test("the tools are the mod's own; nothing beneath gets them", async ($, on) => {
  const seen: string[] = []
  engine(on, e => {
    seen.push(e.tool)
    return { result: '{}' }
  })
  const shown = await start($)
  expect(shown.sections).toEqual(['Widget', 'Storage', 'API'])
  expect(shown.artifacts).toEqual([{ path: 'spec.md', kind: 'other', sections: ['Widget', 'Storage', 'API'] }])
  await call($, { tool: FOCUS, spec_quote: 'kept in memory' })
  expect(seen).toEqual([])
})

test('spec and code side by side when wide, stacked when narrow', async ($, on) => {
  engine(on)
  await start($)
  await call($, {
    tool: FOCUS,
    spec_quote: 'kept in memory',
    code_refs: [{ path: 'store/store.go', symbol: 'store.(*Store).Get' }],
  })

  for (const surface of ['terminal', 'desktop'] as const) {
    const wide = await $.ui.mount(pane(surface, 140))
    expect((await wide.find({ key: 'spec-col' }))?.props.width).toBe('55%')
    expect((await wide.find({ key: 'side-col' }))?.props.width).toBe('45%')
    // The focus box holds the quote's paragraph alone, the quote in bold.
    expect((await wide.find({ key: 'quote-text' }))?.props.text).toBe(
      'Widgets are **kept in memory** and lost on restart.\n\n',
    )
    // Only the Storage section shows, so API's text is not drawn.
    expect((await wide.find({ key: 'after' }))?.props.text).toBe('Another paragraph.\n\n')
    expect((await wide.find({ type: 'Code' }))?.props.startLine).toBe(7)
    // A single-file spec with no doc refs has no file picker.
    expect(await wide.find({ key: 'file' })).toBeUndefined()
    await wide.unmount()

    const narrow = await $.ui.mount(pane(surface, 60))
    expect((await narrow.find({ key: 'spec-col' }))?.props.width).toBe('100%')
    await narrow.unmount()
  }

  // With nothing for the side column, the spec takes the whole width.
  await call($, { tool: FOCUS, spec_quote: 'kept in memory' })
  const alone = await $.ui.mount(pane('desktop', 140))
  expect((await alone.find({ key: 'spec-col' }))?.props.width).toBe('100%')
  expect(await alone.find({ key: 'side-col' })).toBeUndefined()
  // The header names the file and the section on one line; no jump links show.
  expect((await alone.find({ type: 'Text', text: /^› Storage$/ }))?.text).toBe('› Storage')
  expect(await alone.find({ key: 'jumps' })).toBeUndefined()
  await alone.unmount()
})

test('the section picker shows the whole spec or one section', async ($, on) => {
  engine(on)
  await start($)
  await call($, { tool: FOCUS, spec_quote: 'kept in memory' })

  const ui = await $.ui.mount(pane('desktop', 140))
  await ui.select({ key: 'outline', value: '-1' })
  expect(String((await ui.find({ key: 'before' }))?.props.text)).toContain('# Widget')
  expect(String((await ui.find({ key: 'after' }))?.props.text)).toContain('One endpoint.')

  await ui.select({ key: 'outline', value: String(SPEC.indexOf('## API')) })
  expect((await ui.find({ key: 'before' }))?.props.text).toBe('## API\n\nOne endpoint.\n')
  expect(await ui.find({ key: 'quote' })).toBeUndefined()

  await ui.press({ key: 'to-quote' })
  expect(await ui.find({ key: 'quote' })).toBeDefined()
  await ui.unmount()
})

test('code tabs switch the code shown', async ($, on) => {
  engine(on)
  await start($)
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

test('focus fails on a quote, a doc ref or code that is not there', async ($, on) => {
  engine(on)
  await start($)
  expect(failed(await call($, { tool: FOCUS, spec_quote: 'kept on disk' }))).toBe(true)
  expect(failed(await call($, { tool: FOCUS, doc_refs: [{ path: 'docs/design.md', quote: 'no such text' }] }))).toBe(true)
  expect(failed(await call($, { tool: FOCUS, doc_refs: [{ path: 'docs/missing.md', quote: 'anything' }] }))).toBe(true)
  expect(failed(await call($, { tool: FOCUS, code_refs: [{ path: 'store/nope.go', lines: '1' }] }))).toBe(true)
  // spec_section alone is refused, as the bridge refuses it.
  expect(failed(await call($, { tool: FOCUS, spec_section: 'API' }))).toBe(true)
})

// --------------------------------------------------------- doc refs, folders

test('a doc ref opens its file in the pane, with a picker to go back', async ($, on) => {
  engine(on)
  await start($)
  const out = await result(
    call($, { tool: FOCUS, doc_refs: [{ path: 'docs/design.md', quote: 'at most 100 widgets' }] }),
  )
  expect(out.docs).toEqual([{ path: 'docs/design.md', match: 'exact' }])

  const ui = await $.ui.mount(pane('desktop', 140))
  // With no spec quote, the pane shows the doc ref's file and frames its quote.
  expect((await ui.find({ type: 'Text', text: /^docs\/design\.md$/ }))?.text).toBe('docs/design.md')
  expect((await ui.find({ key: 'quote-text' }))?.props.text).toContain('**at most 100 widgets**')
  const picker = await ui.find({ key: 'file' })
  expect(JSON.stringify(picker?.props.options)).toContain('docs/design.md  (quoted)')

  await ui.select({ key: 'file', value: '/repo/docs/spec.md' })
  expect(await ui.find({ key: 'quote' })).toBeUndefined()
  expect(String((await ui.find({ key: 'before' }))?.props.text)).toContain('# Widget')
  await ui.press({ key: 'to-quote' })
  expect((await ui.find({ key: 'quote-text' }))?.props.text).toContain('**at most 100 widgets**')
  await ui.unmount()
})

test('with a spec quote and a doc ref, the spec comes first and a button opens the other', async ($, on) => {
  engine(on)
  await start($)
  await call($, {
    tool: FOCUS,
    spec_quote: 'kept in memory',
    doc_refs: [{ path: 'docs/design.md', quote: 'at most 100 widgets' }],
  })
  const ui = await $.ui.mount(pane('desktop', 140))
  expect((await ui.find({ key: 'quote-text' }))?.props.text).toContain('**kept in memory**')
  await ui.press({ key: 'doc-0' })
  expect((await ui.find({ key: 'quote-text' }))?.props.text).toContain('**at most 100 widgets**')
  await ui.unmount()
})

test('a folder spec lists its markdown files and picks the primary as the bridge does', async ($, on) => {
  engine(on)
  const shown = await start($, 'docs/change')
  expect(shown.primary).toBe('proposal.md')
  expect(shown.artifacts.map((a: { path: string }) => a.path)).toEqual(['proposal.md', 'notes/cache.md'])

  // Doc refs in a folder spec are relative to the folder.
  await call($, { tool: FOCUS, doc_refs: [{ path: 'notes/cache.md', quote: 'at most 100 widgets' }] })
  const ui = await $.ui.mount(pane('desktop', 140))
  expect((await ui.find({ type: 'Text', text: /^notes\/cache\.md$/ }))?.text).toBe('notes/cache.md')
  await ui.unmount()

  // A doc ref outside the folder is refused.
  expect(failed(await call($, { tool: FOCUS, doc_refs: [{ path: '../design.md', quote: 'at most 100 widgets' }] }))).toBe(
    true,
  )
})

test('a path that starts with ~/ is in the home folder', async ($, on) => {
  engine(on)
  const shown = await start($, '~/.claude/specs/repo/draft.md')
  expect(shown.sections).toContain('Storage')
  // Code refs stay relative to the project.
  expect(
    failed(await call($, { tool: FOCUS, spec_quote: 'kept in memory', code_refs: [{ path: 'store/store.go', lines: '7-9' }] })),
  ).toBe(false)
  const ui = await $.ui.mount(pane('desktop', 140))
  expect((await ui.find({ key: 'quote-text' }))?.props.text).toContain('**kept in memory**')
  await ui.unmount()
})

// ----------------------------------------------------------------- questions

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
    text: 'How large is the cache?',
    options: [{ label: '100' }, { label: '1000' }],
    doc_refs: [{ path: 'docs/design.md', quote: 'at most 100 widgets' }],
    code_refs: [{ path: 'store/store.go', symbol: 'store.(*Store).Get' }],
  },
]

// Starts an ask without waiting for it, then mounts the pane once the form shows.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
async function startAsk($: any, clk: { settle: () => Promise<void> }) {
  await start($)
  const pending = call($, { tool: ASK, title: 'Storage', questions: QUESTIONS })
  await clk.settle()
  const ui = await $.ui.mount(pane('desktop', 140))
  return { pending, ui }
}

test('one column repeats the quote in the form and stacks the code under the spec', async ($, on) => {
  const clk = engine(on)
  await start($)
  const pending = call($, { tool: ASK, title: 'Storage', questions: QUESTIONS })
  await clk.settle()
  const ui = await $.ui.mount(pane('desktop', 60))
  expect((await ui.find({ type: 'Text', text: /^“/ }))?.text).toBe('“kept in memory”')
  await ui.press({ key: 'step-1' })
  // No tabs in one column: the form and the code both show.
  expect(await ui.find({ key: 'side-form' })).toBeUndefined()
  expect(await ui.find({ key: 'ask' })).toBeDefined()
  expect((await ui.find({ type: 'Code' }))?.props.startLine).toBe(7)
  await ui.press({ key: 'skip' })
  await clk.advance(POLL)
  expect((await result(pending)).status).toBe('dismissed')
  await ui.unmount()
})

// ask checks once a second: moving the clock past a check lets it see the answer.
const POLL = 1100

test('questions are answered in the pane, each showing its own refs', async ($, on) => {
  const clk = engine(on)
  const { pending, ui } = await startAsk($, clk)
  expect((await ui.find({ key: 'quote-text' }))?.props.text).toContain('**kept in memory**')
  // The recommended option carries a marker of its own.
  expect((await ui.find({ key: 'opt-1' }))?.props.label).toBe('○ Disk  recommended')
  expect((await ui.find({ key: 'opt-0' }))?.props.label).toBe('○ Memory')
  // The quote's caption names the question.
  expect((await ui.find({ type: 'Text', text: /^Question 1/ }))?.text).toBe('Question 1: Where are widgets kept?')
  // Send is the main button only once every question has an answer.
  expect((await ui.find({ key: 'send' }))?.props.variant).toBe('secondary')
  // The description wraps under the button rather than inside it.
  expect(JSON.stringify((await ui.find({ key: 'opt-1' }))?.props)).not.toContain('Survives a restart')
  expect(JSON.stringify(await ui.find({ key: 'opt-desc-1' }))).toContain('Survives a restart')

  // A single choice moves on to q2, whose doc ref and code show.
  await ui.press({ key: 'opt-1' })
  expect((await ui.find({ key: 'quote-text' }))?.props.text).toContain('**at most 100 widgets**')
  // Two columns: the form and q2's code share the side column through tabs.
  expect((await ui.find({ key: 'side-form' }))?.props.variant).toBe('primary')
  expect(await ui.find({ type: 'Code' })).toBeUndefined()
  await ui.press({ key: 'code-0' })
  expect((await ui.find({ type: 'Code' }))?.props.startLine).toBe(7)
  expect(await ui.find({ key: 'ask' })).toBeUndefined()
  await ui.press({ key: 'side-form' })
  expect(await ui.find({ key: 'ask' })).toBeDefined()
  await ui.input({ key: 'notes-q2', text: 'keep it small', kind: 'change' })
  await ui.press({ key: 'opt-0' })
  expect((await ui.find({ key: 'send' }))?.props.variant).toBe('primary')
  await ui.press({ key: 'send' })
  await clk.advance(POLL)

  expect(await result(pending)).toEqual({
    status: 'answered',
    answers: [
      { id: 'q1', selected: ['Disk'], other: null, notes: null },
      { id: 'q2', selected: ['100'], other: null, notes: 'keep it small' },
    ],
  })
  expect(await ui.find({ key: 'ask' })).toBeUndefined()
  await ui.unmount()
})

test('Send asks for every answer; Decide later defers one', async ($, on) => {
  const clk = engine(on)
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

test('after the last answer the pane shows every answer for review', async ($, on) => {
  const clk = engine(on)
  const { pending, ui } = await startAsk($, clk)
  await ui.press({ key: 'opt-1' })
  await ui.input({ key: 'notes-q2', text: 'keep it small', kind: 'change' })
  await ui.press({ key: 'later' })

  expect(await ui.find({ key: 'review' })).toBeDefined()
  expect(await ui.find({ key: 'q-text' })).toBeUndefined()
  expect((await ui.find({ type: 'Text', text: /^→ Disk/ }))?.text).toBe('→ Disk')
  const later = await ui.find({ type: 'Text', text: /^→ Decide later/ })
  expect(later?.text).toBe('→ Decide later')
  expect(later?.props.color).toBe('yellow')
  expect((await ui.find({ type: 'Text', text: /^→ Disk/ }))?.props.color).toBe('cyan')
  expect((await ui.find({ type: 'Text', text: /^Notes: / }))?.text).toBe('Notes: keep it small')

  // A review line goes back to its question; Review returns once all are answered.
  await ui.press({ key: 'review-0' })
  expect(await ui.find({ key: 'review' })).toBeUndefined()
  expect((await ui.find({ key: 'quote-text' }))?.props.text).toContain('**kept in memory**')
  await ui.press({ key: 'step-1' })
  await ui.press({ key: 'to-review' })
  expect(await ui.find({ key: 'review' })).toBeDefined()

  await ui.press({ key: 'send' })
  await clk.advance(POLL)
  const out = await result(pending)
  expect(out.status).toBe('answered')
  expect(out.deferred).toEqual(['q2'])
  await ui.unmount()
})

test('Back to chat copies the questions; Skip dismisses', async ($, on) => {
  const clk = engine(on)
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

test('ask returns pending after the wait, and ask_wait collects the answer', async ($, on) => {
  const clk = engine(on)
  const { pending, ui } = await startAsk($, clk)
  await clk.advance(10 * 60_000 + POLL)
  const first = await result(pending)
  expect(first.status).toBe('pending')
  expect(first.question_set_id).toMatch(/^qs_/)

  await ui.press({ key: 'opt-0' })
  await ui.press({ key: 'opt-0' })
  await ui.press({ key: 'send' })
  const out = await result(call($, { tool: ASK_WAIT, question_set_id: first.question_set_id }))
  expect(out.status).toBe('answered')

  expect(failed(await call($, { tool: ASK_WAIT, question_set_id: 'qs_nope' }))).toBe(true)
  await ui.unmount()
})

test('ask checks its input and its refs as the bridge does', async ($, on) => {
  engine(on)
  await start($)
  expect(failed(await call($, { tool: ASK, questions: [{ id: 'q1', text: 'x', options: [{ label: 'a' }] }] }))).toBe(true)
  expect(failed(await call($, { tool: ASK, questions: [{ ...QUESTIONS[0], spec_quote: 'not in the spec' }] }))).toBe(true)
  expect(
    failed(
      await call($, {
        tool: ASK,
        questions: [{ ...QUESTIONS[1], doc_refs: [{ path: 'docs/design.md', quote: 'not in the note' }] }],
      }),
    ),
  ).toBe(true)
})

test('AskUserQuestion is blocked while a spec is open, and allowed after spec_done', async ($, on) => {
  engine(on)
  // Before any spec, the built-in tool goes on.
  await $.session.start({ cwd: '/repo', surface: 'desktop', isInteractive: true })
  expect(failed(await call($, { tool: 'AskUserQuestion', questions: [] }))).toBe(false)

  await call($, { tool: SPEC_SHOW, path: 'docs/spec.md' })
  expect(failed(await call($, { tool: 'AskUserQuestion', questions: [] }))).toBe(true)

  await call($, { tool: SPEC_DONE, summary: '**Decided**\n\n- memory' })
  const ui = await $.ui.mount(pane('desktop', 140))
  expect(await ui.find({ key: 'done' })).toBeDefined()
  expect((await ui.find({ key: 'summary' }))?.props.text).toContain('memory')
  await ui.unmount()
  expect(failed(await call($, { tool: 'AskUserQuestion', questions: [] }))).toBe(false)
})
