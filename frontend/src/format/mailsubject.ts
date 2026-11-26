// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The reply and forward markers a mail client puts before a subject, colon
 * dropped. A declared mirror of the server's `mailsubject.PrefixWords`;
 * `backend/gates/frontendmailsubject_test.go` fails when the two differ.
 */
export const MAIL_SUBJECT_PREFIXES: readonly string[] = [
  "re",
  "aw",
  "antw",
  "fwd",
  "fw",
  "wg",
];

const leadingPrefixes = new RegExp(
  `^(\\s*(${MAIL_SUBJECT_PREFIXES.join("|")})\\s*:)+\\s*`,
  "i",
);

/** The subject without the reply and forward markers in front of it. */
export function withoutSubjectPrefixes(subject: string): string {
  return subject.replace(leadingPrefixes, "");
}

/**
 * The forward markers alone. A declared mirror of `mailsubject.ForwardPrefixes`,
 * held by the same gate.
 */
export const MAIL_FORWARD_PREFIXES: readonly string[] = ["fwd", "fw", "wg"];

// The outermost prefix is the first one. The spaces are those Go's `\s` reads,
// and U+00A0, so both sides read one subject alike.
const outerForward = new RegExp(
  `^[\\t\\n\\f\\r \\u00a0]*(${MAIL_FORWARD_PREFIXES.join("|")})[\\t\\n\\f\\r \\u00a0]*:`,
  "i",
);

/**
 * Whether the subject's outermost prefix marks a forward. "Re: Fwd:" answers
 * a forward, so it is a reply.
 */
export function isForwardSubject(subject: string): boolean {
  return outerForward.test(subject);
}
