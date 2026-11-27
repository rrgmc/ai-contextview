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

/** A passage of a document, from doc_refs: a quote, or a line range. */
export type DocRefView = QuoteView & {
  /** The file, absolute. */
  path: string
  /** Offset of the heading of the quote's section in that file, or -1. */
  shownAt: number
  /** The line range of a ref given by lines (1-based), or 0 for a quote. */
  startLine: number
  endLine: number
}

/** What the model last showed: the spec, the focused passage and the code. */
export type SpecView = QuoteView & {
  /** The primary file, absolute. */
  specPath: string
  title: string
  /** The folder the spec's paths are relative to: the folder given, or the file's own. */
  dir: string
  isDir: boolean
  /** Every markdown file of a folder spec, absolute, the primary first; for a file, that file. */
  files: string[]
  /** Files of a folder spec the last spec_show named as changed, absolute. */
  changedFiles: string[]
  /** Quotes of the last focus in files other than the primary. */
  docs: DocRefView[]
  /** The whole primary spec file, as read at the last spec_show, focus or ask. */
  text: string
  changedSections: string[]
  /** How the quote was found: exact, normalized, or none. */
  match: string
  code: CodeView[]
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
  docs: DocRefView[]
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
  /** True while the pane shows every answer for review instead of one question. */
  review: boolean
  drafts: Record<string, Draft>
  error: string
  /** Null while the questions wait for the person. */
  result: AskOutput | null
}

/** What the person picked in the pane. */
export type PaneUi = {
  /** The file shown, absolute; empty for the primary file. */
  file: string
  /** Offset of the heading of the section shown, or -1 for the whole spec. */
  shown: number
  /** Index of the code tab shown; -1 for the question form, which shares the side column in two columns. */
  codeTab: number
}

/** How the working document is picked from written files (docs/07-layers.md). */
export type WorkDoc = {
  /** Claude's own spec_show, which always wins. */
  explicit: boolean
  /** Spec mode: ask mode that expects a spec. */
  spec: boolean
  /** Spec mode's absolute glob patterns. */
  paths: string[]
  /** A skill whose name looks like a spec skill started. */
  specSkill: boolean
  /** The working document picked from a write, absolute; cleared when ask mode ends. */
  doc: string
  /**
   * The spec file the pane shows because a write picked it, absolute; kept after
   * ask mode ends, so that file does not count as a spec Claude opened.
   */
  autoShown: string
  /**
   * doc was shown with spec_show provisional: the first written file that would
   * become the working document replaces it.
   */
  provisional: boolean
  /** Writes per candidate file, absolute. */
  edits: Record<string, number>
}

declare module 'claude-code' {
  interface PluginState {
    'ai-specview': {
      view: Shaped<SpecView | null>
      ui: Shaped<PaneUi>
      ask: Shaped<AskView | null>
      mode: Shaped<boolean>
      work: Shaped<WorkDoc>
    }
  }
}
