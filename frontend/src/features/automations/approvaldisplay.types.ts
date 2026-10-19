// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../../i18n/en";

// The shapes of a kind's display declaration and of what resolving it yields.
// They sit apart from the catalogue in approvaldisplay.ts, which re-exports
// them, so a module that only resolves fields needs no import of the catalogue.

/** What one payload field is, as a kind declares it. */
export type DisplayField = {
  readonly field: string;
  /** What the field is CALLED. A wire name is a payload path, not a caption. */
  readonly label: MessageKey;
  /**
   * How to read the value.
   *
   * `prose` is a sentence the server wrote for a human. `date` is a date-only
   * wire string, rendered on the reader's calendar. `enum` looks the value up
   * in `optionLabels`, because a raw enum on screen is a wire token wearing a
   * caption. `text` is a value that is already a word: a name, an address.
   */
  readonly as: "prose" | "text" | "date" | "enum";
  /**
   * Promotes this field to the card's lead sentence rather than a labelled
   * fact. At most one per kind. Only a field that answers "why am I being
   * asked this?" leads: the reason, never the value being proposed.
   */
  readonly lead?: true;
  /** What each enum value is CALLED. Required in spirit for `as: "enum"`. */
  readonly optionLabels?: Readonly<Record<string, MessageKey>>;
};

/** A DisplayField with its label and value resolved for one payload. */
export type ResolvedDisplayField = Readonly<{
  field: string;
  label: string;
  value: string | null;
  lead?: true;
}>;
