// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale } from "../i18n";
import { type FieldLabel, fieldCheckSentence } from "./faultsentence";

// One @ between a non-empty local part and domain, with no whitespace. Looser
// than the server's net/mail parse, so a form never refuses what it accepts.
const EMAIL_SHAPE = /^[^\s@]+@[^\s@]+$/;

/** Whether typed text can be an email address; the server has the last word. */
export function looksLikeEmail(text: string): boolean {
  return EMAIL_SHAPE.test(text.trim());
}

/**
 * Why typed text is not an email address, in the words of the server's own
 * refusal, or undefined for an address or no text. A form's own check: the
 * browser's would answer in the browser's language.
 */
export function emailFault(
  text: string,
  locale: Locale,
  label?: FieldLabel,
): string | undefined {
  const typed = text.trim();
  return typed === "" || looksLikeEmail(typed)
    ? undefined
    : fieldCheckSentence("email_malformed", locale, label);
}
