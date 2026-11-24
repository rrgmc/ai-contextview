export type Mode = 'both' | 'pane' | 'browser'

/** One block of code the pane shows. */
export type CodeView = {
  path: string
  label: string
  startLine: number
  source: string
}

/** What the model last showed: the spec, the focused passage and the code. */
export type SpecView = {
  specPath: string
  title: string
  /** The whole primary spec file, as read at the last spec_show or focus. */
  text: string
  changedSections: string[]
  /** The quoted passage, when a focus named one, and where it sits in text (-1 when not found). */
  quote: string
  quoteStart: number
  quoteEnd: number
  quoteSection: string
  /** How the quote was found: exact, normalized, or none. */
  match: string
  code: CodeView[]
  /** Where the last update came from: the MCP bridge or the pane alone. */
  via: 'bridge' | 'pane'
  error: string
}

/** What the person picked in the pane. */
export type PaneUi = {
  /** Offset of the heading of the section shown, or -1 for the whole spec. */
  shown: number
  /** Index of the code tab shown. */
  codeTab: number
}

declare module 'claude-code' {
  interface PluginState {
    'ai-specview-pane': {
      view: Shaped<SpecView | null>
      mode: Shaped<Mode | null>
      ui: Shaped<PaneUi>
    }
  }
}
