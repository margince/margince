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
