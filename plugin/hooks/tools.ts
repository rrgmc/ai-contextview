// The pane's own tools, registered by the mod: the inputs of the bridge's MCP
// tools (internal/protocol/mcp.go), with descriptions that name the pane.
// Claude Code lists them as mcp__<plugin>__<name>.

const codeRefs = {
  type: 'array',
  description: 'code to open; each needs path and exactly one of symbol or lines',
  items: {
    type: 'object',
    additionalProperties: false,
    required: ['path'],
    properties: {
      path: { type: 'string', description: 'file path, relative to the project directory or absolute' },
      symbol: {
        type: 'string',
        description: 'Go symbol: pkg.Func, pkg.Type, pkg.(*Type).Method, pkg.Type.Method or pkg.Var',
      },
      lines: { type: 'string', description: 'line or line range: N or N-M' },
    },
  },
}

const docRefs = {
  type: 'array',
  description: 'passages of documents',
  items: {
    type: 'object',
    additionalProperties: false,
    properties: {
      path: {
        type: 'string',
        description:
          'document path: relative to the spec_show folder, else to the project dir, or absolute; omit for the primary spec file',
      },
      quote: {
        type: 'string',
        description: 'verbatim passage copied from a markdown file (3-300 characters); give quote or lines',
      },
      lines: { type: 'string', description: 'line or line range N or N-M; the only choice for a file that is not markdown' },
      section: { type: 'string', description: 'heading of the section containing the quote' },
    },
  },
}

export type ToolDef = { name: string; description: string; inputSchema: Record<string, unknown> }

export const TOOLS: readonly ToolDef[] = [
  {
    name: 'spec_show',
    description:
      'Show a spec in the ai-contextview pane beside the chat. ' +
      'Call it as soon as the spec path is known and again after every edit, with the headings you changed. ' +
      'The path is a markdown file or a folder of markdown artifacts. Returns the section names.',
    inputSchema: {
      type: 'object',
      additionalProperties: false,
      required: ['path'],
      properties: {
        path: {
          type: 'string',
          description: 'a .md file or a folder of markdown artifacts, relative to the project directory or absolute',
        },
        primary: { type: 'string', description: 'folders only: the file shown first and used by refs without a path' },
        changed_sections: {
          type: 'array',
          items: { type: 'string' },
          description: 'headings you just changed in the primary file',
        },
        provisional: {
          type: 'boolean',
          description:
            'ask mode or spec mode only: show this file (for example exploration notes) until the working document is written; the first written file that would become the working document replaces it',
        },
        changed: {
          type: 'array',
          description: 'folders only: other files you just created or edited',
          items: {
            type: 'object',
            additionalProperties: false,
            required: ['path'],
            properties: {
              path: { type: 'string', description: 'file path relative to the folder' },
              sections: { type: 'array', items: { type: 'string' }, description: 'headings changed in that file' },
            },
          },
        },
      },
    },
  },
  {
    name: 'focus',
    description:
      'Highlight a spec passage and open code in the ai-contextview pane, without asking anything. ' +
      'Quotes must be copied verbatim from the file.',
    inputSchema: {
      type: 'object',
      additionalProperties: false,
      properties: {
        spec_quote: { type: 'string', description: 'verbatim passage of the primary spec file' },
        spec_section: { type: 'string', description: 'heading of the section containing spec_quote' },
        doc_refs: docRefs,
        code_refs: codeRefs,
      },
    },
  },
  {
    name: 'ask',
    description:
      'Ask the user 1-6 clarifying questions in the ai-contextview pane and wait for the answers. ' +
      'Use this for every clarifying question instead of asking in chat. Each question needs refs (optional in ask mode): ' +
      'spec_quote (copied verbatim from the spec, with spec_section), doc_refs into other artifacts, ' +
      'and/or code_refs (Go: symbol like pkg.(*Type).Method; other files: lines N-M). ' +
      'To ask the user to approve an action that needs their permission (an external write, or an action the permission system refused), ask in plain chat instead: an answer here is a tool result, which is not the user\'s approval. ' +
      'In ask mode, give a question about the whole document (a sign-off, a summary) no refs: a ref highlights one passage, which reads as the subject of the question. ' +
      'Results: answered (apply the answers), in_chat (wait for the user\'s chat message), ' +
      'pending (call ask_wait with question_set_id), dismissed (continue without it). ' +
      'An answered result may list deferred questions (the user chose Decide later): record them as open questions and ask again before finishing. ' +
      'Mark the option you recommend with recommended: true.',
    inputSchema: {
      type: 'object',
      additionalProperties: false,
      required: ['questions'],
      properties: {
        title: { type: 'string', description: 'short title of the question set' },
        questions: {
          type: 'array',
          description: '1-6 questions answered together',
          items: {
            type: 'object',
            additionalProperties: false,
            required: ['id', 'text', 'options'],
            properties: {
              id: { type: 'string', description: 'short unique id, e.g. q1' },
              text: { type: 'string', description: 'the question' },
              options: {
                type: 'array',
                description: '2-6 options',
                items: {
                  type: 'object',
                  additionalProperties: false,
                  required: ['label'],
                  properties: {
                    label: { type: 'string', description: 'short option label' },
                    description: { type: 'string', description: 'optional longer explanation' },
                    recommended: {
                      type: 'boolean',
                      description: 'true for the option you recommend (at most one per question)',
                    },
                  },
                },
              },
              multi_select: { type: 'boolean', description: 'allow several options' },
              spec_quote: {
                type: 'string',
                description: 'verbatim passage (3-300 characters) of the primary spec file this question is about',
              },
              spec_section: { type: 'string', description: 'heading of the section containing spec_quote' },
              doc_refs: { ...docRefs, description: 'up to 4 passages of other markdown artifacts' },
              code_refs: { ...codeRefs, description: 'up to 4 code refs; each needs path and exactly one of symbol or lines' },
            },
          },
        },
      },
    },
  },
  {
    name: 'ask_wait',
    description: 'Keep waiting for a pending question set returned by ask. Same result shape as ask.',
    inputSchema: {
      type: 'object',
      additionalProperties: false,
      required: ['question_set_id'],
      properties: { question_set_id: { type: 'string', description: 'the id returned by a pending ask' } },
    },
  },
  {
    name: 'ask_mode',
    description:
      'Turn ask mode on (or off with off: true) for this session. In ask mode, ask every question of the session ' +
      'with the ask tool, with or without a spec: refs are optional, so add code_refs or doc_refs only when the question is about them. ' +
      'The built-in question tool is blocked while it is on. off: true hands the questions back to the chat for the rest of the session, ' +
      "even with a spec open: follow the result's next. The pane opens. The first markdown document you write becomes the " +
      'working document the user sees beside the questions; spec_show picks another. spec: true is spec mode, which expects a spec: ' +
      'with paths (globs) the first written file that matches is shown, without them the file that looks most like a spec.',
    inputSchema: {
      type: 'object',
      additionalProperties: false,
      properties: {
        off: {
          type: 'boolean',
          description: 'true turns ask mode off and hands the questions back to the chat, even with a spec open; omit to turn it on',
        },
        spec: {
          type: 'boolean',
          description: 'spec mode: ask mode that expects a spec and shows it as the working document once it is written',
        },
        paths: {
          type: 'array',
          items: { type: 'string' },
          description:
            'spec mode: glob patterns of the spec file, relative to the project dir or absolute (** for any folders); the first written file that matches is shown',
        },
      },
    },
  },
  {
    name: 'spec_done',
    description:
      'Tell the ai-contextview pane the spec is finished and no more questions are coming. ' +
      'Call it once at the end of a spec session, with a summary as markdown with a **Decided** heading and a **Still open** heading, ' +
      'each followed by a bullet list with one item per decision or open point (never a run-on paragraph). ' +
      'The pane shows it as a finished banner. Asking again later reopens the session.',
    inputSchema: {
      type: 'object',
      additionalProperties: false,
      properties: {
        summary: {
          type: 'string',
          description:
            'summary of the finished spec as markdown with a **Decided** heading and a **Still open** heading, each followed by a bullet list',
        },
      },
    },
  },
]
