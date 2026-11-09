// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** Space (U+0020) through tilde (U+007E): every key a US layout types alone. */
const PRINTABLE_ASCII_COUNT = 95;

/** Every printable ASCII character, as `KeyboardEvent.key` reports it. */
export const PRINTABLE_KEYS: readonly string[] = Array.from(
  { length: PRINTABLE_ASCII_COUNT },
  (_, at) => String.fromCodePoint(0x20 + at),
);
