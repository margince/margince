// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One @ between a non-empty local part and domain, with no whitespace. Looser
// than the server's net/mail parse, so a form never refuses what it accepts.
const EMAIL_SHAPE = /^[^\s@]+@[^\s@]+$/;

/** Whether typed text can be an email address; the server has the last word. */
export function looksLikeEmail(text: string): boolean {
  return EMAIL_SHAPE.test(text.trim());
}
