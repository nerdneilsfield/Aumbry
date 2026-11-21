import DOMPurify from 'dompurify'

/** Search snippets are server-escaped HTML with <mark> only; enforce that before v-html. */
export const snippetHtml = (html: string) => DOMPurify.sanitize(html, { ALLOWED_TAGS: ['mark'], ALLOWED_ATTR: [] })
