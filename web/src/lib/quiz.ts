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

/** SHA-256 (Web Crypto) of the signature, hex. '' when Web Crypto is unavailable (then stored answers are trusted as they are). */
export async function quizFingerprint(q: Element): Promise<string> {
  if (!globalThis.crypto?.subtle) return ''
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(quizSignature(q)))
  return Array.from(new Uint8Array(digest), (b) => b.toString(16).padStart(2, '0')).join('')
}

export const sameSet = (a: string[], b: string[]) => a.length === b.length && a.every((x) => b.includes(x))
