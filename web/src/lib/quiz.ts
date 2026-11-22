import { children } from '@/lib/xml'
import { quizKey, safeId } from '@/lib/anchor'

export { quizKey }

/** <quiz> helpers shared by the renderer and the validator. Option keys are the option's index as a string. */
export const quizQuestion = (q: Element) => children(q).find((c) => c.tagName === 'question')?.textContent?.trim() ?? ''
export const quizOptions = (q: Element) => children(q).filter((c) => c.tagName === 'option')
export const quizCorrect = (q: Element) => quizOptions(q).flatMap((o, i) => (o.getAttribute('correct') === 'true' ? [String(i)] : []))
/** INVALID_QUIZ: valid id, non-empty <question>, at least 2 options, and at least 1 correct="true". */
export const quizValid = (q: Element) => !!safeId(q.getAttribute('id')) && !!quizQuestion(q) && quizOptions(q).length >= 2 && quizCorrect(q).length >= 1
export const quizMulti = (q: Element) => quizCorrect(q).length > 1

/** Characters of a <quiz> that decide whether a stored answer still applies: question, option order and texts, correct set. */
export const quizSignature = (q: Element) =>
  JSON.stringify([quizQuestion(q), quizOptions(q).map((o) => o.textContent?.trim() ?? ''), quizCorrect(q)])

/**
 * SHA-256 of the signature, hex. Web Crypto where it exists; `crypto.subtle` is missing in non-secure contexts (plain http on a
 * host other than localhost), where @noble/hashes (pure JS, loaded on demand) computes the same digest. Either way the
 * fingerprint is real: a changed quiz never inherits an old answer.
 */
export async function quizFingerprint(q: Element): Promise<string> {
  const data = new TextEncoder().encode(quizSignature(q))
  if (globalThis.crypto?.subtle) return toHex(new Uint8Array(await crypto.subtle.digest('SHA-256', data)))
  const { sha256 } = await import('@noble/hashes/sha2.js')
  return toHex(sha256(data))
}

const toHex = (bytes: Uint8Array) => Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')

export const sameSet = (a: string[], b: string[]) => a.length === b.length && a.every((x) => b.includes(x))
