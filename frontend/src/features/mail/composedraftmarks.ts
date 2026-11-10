// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import type { ProjectScope } from "../../design-system/projectpicker";
import type { Locale } from "../../i18n/locale";
import type { DraftProvenance } from "./composedraftcall";

type DraftReason = components["schemas"]["AccountDraftReason"];

/**
 * What marks a body as the model's: its reference, disclosure and reasons, and
 * the language the rep had chosen for it.
 */
export type DraftMarks = Readonly<{
  draftRef: string | null;
  provenance: DraftProvenance | null;
  reasoning: DraftReason[];
  scope: ProjectScope | undefined;
  language: Locale | null;
}>;

export const plainText = (text: string) => text.replace(/\s+/g, " ").trim();

const wordsOf = (text: string) =>
  text
    .normalize("NFC")
    .toLowerCase()
    .match(/[\p{L}\p{M}\p{N}]+/gu) ?? [];

// The fewest served words in a row that count as the model's passage.
const PASSAGE_WORDS = 5;

// An undo or paste brings the draft back as a passage, edits and all. Words
// merely shared with it, in any order, are not that passage.
export function holdsServedWords(body: string, served: string): boolean {
  const servedWords = wordsOf(served);
  const held = ` ${wordsOf(body).join(" ")} `;
  for (let at = 0; at + PASSAGE_WORDS <= servedWords.length; at += 1) {
    const passage = servedWords.slice(at, at + PASSAGE_WORDS).join(" ");
    if (held.includes(` ${passage} `)) return true;
  }
  return false;
}

// One key typed or deleted: the reader writing, never an undo that lands on
// a body held before.
export function oneKeystroke(before: string, after: string): boolean {
  // By code point, so an emoji typed is one key and not two.
  const [short, long] = [Array.from(before), Array.from(after)].sort(
    (a, b) => a.length - b.length,
  );
  if (!short || !long || long.length - short.length !== 1) return false;
  let at = 0;
  while (at < short.length && short[at] === long[at]) at += 1;
  return long.slice(at + 1).join("") === short.slice(at).join("");
}

// Enough for an undo several drafts deep, and small enough that a held
// Backspace on a long body cannot grow it without end.
export const REMEMBERED_BODIES = 20;

/**
 * The bodies this composer has held, by their plain text, and the marks each
 * carried. An undo lands on one of them, and takes its marks with it.
 */
export class MarksMemory {
  private readonly bodies = new Map<string, DraftMarks | null>();

  remember(text: string, marks: DraftMarks | null): void {
    const key = plainText(text);
    if (key === "") return;
    this.bodies.delete(key);
    this.bodies.set(key, marks);
    for (const oldest of this.bodies.keys()) {
      if (this.bodies.size <= REMEMBERED_BODIES) break;
      this.bodies.delete(oldest);
    }
  }

  /** The marks a remembered body carried, or `undefined` for a new body. */
  marksOf(text: string): DraftMarks | null | undefined {
    return this.bodies.get(plainText(text));
  }

  forget(): void {
    this.bodies.clear();
  }
}
