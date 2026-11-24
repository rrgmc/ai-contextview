import { expect, test } from 'claude-code/testing'

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

// The engine beneath the mod: a session in /repo with two files.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function engine(on: any, bridge: (e: any) => unknown) {
  on('command.register', () => ({ value: { command: 'x' } }))
  on('ui.open', () => ({ value: { isPlaced: true } }))
  on('ui.scroll', () => ({ value: {} }))
  on('clock.after', () => ({ value: {} }))
  on('session.start', (_: unknown, e: { cwd: string }) => ({ cwd: e.cwd }))
  on('fs.read', (_: unknown, e: { path: string }) => ({ value: FILES[e.path] }))
  on('fs.exists', (_: unknown, e: { path: string }) => ({ value: e.path in FILES }))
  on('fs.stat', () => ({ value: { kind: 'file', size: 1, mtimeMs: 0, isLink: false } }))
  on('tool.call', (_: unknown, e: unknown) => bridge(e))
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
    expect((await wide.find({ key: 'code-col' }))?.props.width).toBe('45%')
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
    spec_section: 'API',
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

test('pane mode sends the bridge the spec once, before the first ask', { options: { mode: 'pane' } }, async ($, on) => {
  const seen: string[] = []
  engine(on, (e: { tool: string; path?: string }) => {
    seen.push(`${e.tool}:${e.path ?? ''}`)
    return { result: '{"status":"answered"}', text: '{}' }
  })
  await $.session.start({ cwd: '/repo', surface: 'desktop', isInteractive: true })
  await call($, { tool: SPEC_SHOW, path: 'docs/spec.md' })
  await call($, { tool: ASK, questions: [] })
  await call($, { tool: ASK, questions: [] })
  expect(seen).toEqual([`${SPEC_SHOW}:docs/spec.md`, `${ASK}:`, `${ASK}:`])
})
