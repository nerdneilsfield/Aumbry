import TurndownService from 'turndown'

const td = new TurndownService()

/** Plain text as one line of Markdown text: turndown's own escaping, newlines folded to spaces. */
export const mdLine = (s: string) => td.escape(s).replace(/\s*\n\s*/g, ' ').trim()

/** Plain text as a GFM table cell: `mdLine`, plus the pipe, which would end the cell. */
export const mdCell = (s: string) => mdLine(s).replace(/\|/g, '\\|')
