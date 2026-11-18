// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The password rule the client states, in one place.
 *
 * It was in three: `passwordcard.tsx`, `setupclaim.tsx` and `auth.tsx` each
 * declared `MIN_PASSWORD = 12`, each commented as "the floor the server applies,
 * restated". Three copies of a number the server owns is a lesser problem than
 * what sat under them — the three did not agree on what a character IS.
 * `auth.tsx` measured `password.length`, which counts UTF-16 code units, while
 * the other two measured `[...password].length`, which counts code points. A
 * password carrying an emoji or any astral character therefore passed on the
 * reset screen and was refused on the change screen, or the reverse, depending
 * on which side of twelve it landed. The server has one rule; the client had
 * two, and neither screen could see the other.
 *
 * Code points is the reading kept, because it is what a contact means by "twelve
 * characters" — a single emoji is one character to the reader who typed it, and
 * counting it as two lets a password that LOOKS eleven characters long pass.
 */
export const MIN_PASSWORD = 12;

/** How long this password is, counted the way the reader counted it. */
export function passwordLength(password: string): number {
  return [...password].length;
}

/**
 * The ceiling the server applies, in the same code-point count as the floor.
 * The server rule is `passwordRuleError` in
 * backend/internal/modules/identity/service.go; a gate holds both numbers and
 * the refusals together.
 */
export const MAX_PASSWORD = 256;

/** Why a password is refused before the server is asked, as the i18n key of the
 *  refusal. Empty is unanswered, not refused: a form that scolds a field nobody
 *  has typed in yet is reporting on itself rather than on the reader. The email
 *  rule is the server's alone, because only it knows the account. */
export type PasswordFault =
  | "password.tooShort"
  | "password.tooLong"
  | "password.blank"
  | "password.repeated";

// White space as the server reads it (Go's unicode.IsSpace), spelled out because
// String.trim also strips U+FEFF and not U+0085. passwordrule.test.ts holds this
// to the corpus the server test reads.
const BLANK =
  /^[\t-\r \u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+$/;

export function passwordFault(password: string): PasswordFault | null {
  if (password.length === 0) {
    return null;
  }
  const length = passwordLength(password);
  if (length < MIN_PASSWORD) {
    return "password.tooShort";
  }
  if (length > MAX_PASSWORD) {
    return "password.tooLong";
  }
  if (BLANK.test(password)) {
    return "password.blank";
  }
  if (new Set(password).size === 1) {
    return "password.repeated";
  }
  return null;
}
