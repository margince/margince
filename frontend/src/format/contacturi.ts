// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The one place that decides whether an address or a number may become a
// `mailto:` or a `tel:` link.
//
// Both values are untrusted record data: a connector wrote them, a crawl wrote
// them, or a person pasted them. A `mailto:` built by concatenation is an
// injection surface — `?subject=`, `&cc=` and a `%0a` in the address all reach
// the reader's mail client as header fields — and a `tel:` built the same way
// dials whatever the string held. So each is admitted only in the shape it can
// honestly have, and everything else stays text. What a surface DRAWS for a
// refused value is the caller's decision (ContactLink keeps the fact and drops
// the link); the shapes are not.

// A mailbox with one `@` between two non-empty halves, and none of the
// characters that would let the address carry a second header or escape the
// scheme. Deliberately stricter than the addressing RFCs: a quoted local part
// is legal mail and a wrong thing to hand a link to.
const MAILBOX = /^[^\s@?#&%<>"'/\\]+@[^\s@?#&%<>"'/\\]+\.[^\s@?#&%<>"'/\\.]+$/;

// The characters people put between digits when they write a number down.
const NUMBER_PUNCTUATION = /[\s().\-/]/g;

/** mailtoUri is the `mailto:` for an address, or null when it is not one. */
export function mailtoUri(address: string): string | null {
  const trimmed = address.trim();
  return MAILBOX.test(trimmed) ? `mailto:${trimmed}` : null;
}

/**
 * telUri is the `tel:` for a phone number, or null when the value does not
 * reduce to one. The punctuation a person writes between digits is dropped;
 * an optional leading `+` and at least three digits are what remain.
 */
export function telUri(number: string): string | null {
  const digits = number.trim().replace(NUMBER_PUNCTUATION, "");
  return /^\+?\d{3,}$/.test(digits) ? `tel:${digits}` : null;
}
