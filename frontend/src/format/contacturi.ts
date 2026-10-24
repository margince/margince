// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type ContactPhone = components["schemas"]["ContactPhone"];

// The one place that decides whether an address may be written to or a number
// may become a `tel:` link.
//
// Both values are untrusted record data: a connector wrote them, a crawl wrote
// them, or a contact pasted them. An address handed on by concatenation is an
// injection surface — `?subject=`, `&cc=` and a `%0a` in it all reach whatever
// consumes it as header fields — and a `tel:` built the same way dials whatever
// the string held. So each is admitted only in the shape it can honestly have,
// and everything else stays text. What a surface DRAWS for a refused value is
// the caller's decision (ContactLink keeps the fact and drops the action); the
// shapes are not.

// A mailbox with one `@` between two non-empty halves, and none of the
// characters that would let the address carry a second header, a second
// recipient (`,` and `;` separate mailboxes in an address list) or another
// scheme. Deliberately stricter than the addressing RFCs: a quoted local part
// is legal mail and a wrong thing to hand on unread.
const MAILBOX =
  /^[^\s@?#&%<>"'/\\,;:]+@[^\s@?#&%<>"'/\\,;:]+\.[^\s@?#&%<>"'/\\,;:.]+$/;

// The characters contacts put between digits when they write a number down.
const NUMBER_PUNCTUATION = /[\s().\-/]/g;

/** mailbox is the address itself, trimmed, or null when it is not one. */
export function mailbox(address: string): string | null {
  const trimmed = address.trim();
  return MAILBOX.test(trimmed) ? trimmed : null;
}

/**
 * telUri is the `tel:` for a phone number, or null when the value does not
 * reduce to one. The punctuation a contact writes between digits is dropped;
 * an optional leading `+` and at least three digits are what remain.
 */
export function telUri(number: string): string | null {
  const digits = number.trim().replace(NUMBER_PUNCTUATION, "");
  return /^\+?\d{3,}$/.test(digits) ? `tel:${digits}` : null;
}

// Declared mirror of values.E164Pattern in backend/internal/shared/kernel/values/phone.go.
const E164 = /^\+([1-9]\d{7,14})$/;

/** The chat with a number in each messenger, keyed by messenger. */
export type MessengerUris = Readonly<{
  whatsapp: string;
  telegram: string;
  zalo: string;
}>;

/**
 * messengerUris is the chat address for a number in each messenger, or null
 * unless the number is E.164. A messenger finds a contact only by country code.
 */
export function messengerUris(number: string): MessengerUris | null {
  const digits = E164.exec(number.trim().replace(NUMBER_PUNCTUATION, ""))?.[1];
  if (digits === undefined) {
    return null;
  }
  return {
    whatsapp: `https://wa.me/${digits}`,
    telegram: `https://t.me/+${digits}`,
    zalo: `https://zalo.me/${digits}`,
  };
}

// Declared mirror of contactaddress.ReachablePhoneOrder: is_primary DESC, then the server's order.
export function primaryPhone(
  phones: readonly ContactPhone[] | undefined,
): ContactPhone | undefined {
  const live = (phones ?? []).filter((row) => row.phone !== "");
  return live.find((row) => row.is_primary) ?? live[0];
}
