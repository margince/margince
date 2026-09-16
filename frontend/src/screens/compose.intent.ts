// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * What the composer OPENS WITH, where the caller knows why it was opened.
 *
 * The steer field is read by a language model and shown to a rep, so it takes
 * a sentence rather than a token. Where the caller can also say WHICH thing,
 * that rides in the same field: a reason firing on one of several open threads
 * asks the composer to "reply to their last message" and leaves the drafter to
 * guess which one.
 *
 * Spelled once because two surfaces seed it — a contact's moment action and
 * the queue's prepared reply — and a phrase joined one way here and another
 * way there is two answers to what the model is being told.
 */
export function intentAbout(phrase: string, about?: string | null): string {
  const named = about?.trim();
  return named ? `${phrase}: ${named}` : phrase;
}
