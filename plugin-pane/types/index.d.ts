export type Mode = 'both' | 'pane' | 'browser'

/** One block of code the pane shows. */
export type CodeView = {
  path: string
  label: string
  startLine: number
  source: string
}

/** A quote located in the spec: where it sits (-1 when not found) and its section. */
export type QuoteView = {
  quote: string
  quoteStart: number
  quoteEnd: number
  quoteSection: string
}

/** What the model last showed: the spec, the focused passage and the code. */
export type SpecView = QuoteView & {
  specPath: string
  title: string
  /** The whole primary spec file, as read at the last spec_show, focus or ask. */
  text: string
  changedSections: string[]
  /** How the quote was found: exact, normalized, or none. */
  match: string
  code: CodeView[]
  /** Where the last update came from: the MCP bridge or the pane alone. */
  via: 'bridge' | 'pane'
  error: string
  /** True after spec_done, until the next spec_show or ask. */
  isDone: boolean
  /** The spec_done summary, as markdown. */
  summary: string
}

/** One option of a question. */
export type OptionView = { label: string; description: string; recommended: boolean }

/** One question of a pending ask, with its refs resolved. */
export type QuestionView = QuoteView & {
  id: string
  text: string
  options: OptionView[]
  multi: boolean
  allowOther: boolean
  code: CodeView[]
}

/** What the person has entered for one question so far. */
export type Draft = { selected: string[]; other: string; notes: string; later: boolean }

/** One answer, as the ask tool returns it. */
export type Answer = {
  id: string
  selected: string[]
  other: string | null
  notes: string | null
  deferred?: boolean
}

/** The ask tool's result. */
export type AskOutput = {
  status: 'answered' | 'in_chat' | 'pending' | 'dismissed'
  answers?: Answer[]
  deferred?: string[]
  question_set_id?: string
  message?: string
}

/** A question set the pane shows, and its outcome once the person acted. */
export type AskView = {
  qsId: string
  title: string
  questions: QuestionView[]
  index: number
  drafts: Record<string, Draft>
  error: string
  /** Null while the questions wait for the person. */
  result: AskOutput | null
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
      ask: Shaped<AskView | null>
    }
  }
}
