// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// German copy speaks to its reader as du, and two checks hold that.
//
// 1. No value addresses the reader formally. Capitalised Sie / Ihr… is that
//    address, and only the key families an outside reader opens are exempt.
//    "Sie" opening a sentence as she or they is rephrased, never waived.
// 2. A du-pronoun is capitalised only where a sentence begins. Mid-sentence it
//    is the formal register wearing the informal word.
// 3. The buyer's page speaks Sie throughout, so every key its files name from
//    outside the outsider families carries no du-pronoun.
//
// The mechanics of German copy are the copy-style-de rule's; both read the
// catalogs it derives.

import {
  LETTER,
  READ_BY_AN_OUTSIDER,
  readByAnOutsider,
} from "../../scripts/lib/german-catalogs";
import {
  catalogEntries,
  type LocaleCopy,
} from "../../scripts/lib/locale-catalogs";
import type { CensusRule } from "../tree";
import { coreCatalog, germanCopies } from "./copy-style-de.census";

// Longest first: the alternation is what gets reported, and "Ihr" would match
// the head of "Ihrem" and name the wrong word in the finding.
const FORMAL = new RegExp(
  `(?<![${LETTER}])(Ihnen|Ihres|Ihrem|Ihren|Ihrer|Ihre|Ihr|Sie)(?![${LETTER}])`,
  "g",
);

const DU_ANY_CASE = new RegExp(
  `(?<![${LETTER}])(deines|deinem|deinen|deiner|deine|dein|dich|dir|du)(?![${LETTER}])`,
  "i",
);

// The files of the page a buyer opens. Stories and tests render planted copy.
const BUYER_PAGE = /^src\/features\/deals\/buyerroom[a-z]*\.tsx$/;
const QUOTED = /"([a-z][A-Za-z0-9]*\.[A-Za-z0-9_.]+)"/g;

/** Every du-pronoun the buyer's page would show through a key it names. */
export function buyerPageFindings(
  copies: readonly LocaleCopy[],
  pages: readonly Readonly<{ rel: string; text: string }>[],
): string[] {
  const entries = catalogEntries(copies);
  return pages.flatMap((page) => {
    const named = new Set([...page.text.matchAll(QUOTED)].map((m) => m[1]));
    return entries
      .filter(([, key]) => named.has(key) && !readByAnOutsider(key))
      .filter(([, , value]) => DU_ANY_CASE.test(value))
      .map(
        ([source, key, value]) =>
          `says du on the buyer's page: ${page.rel} names ${source} ${key}: ${value}`,
      );
  });
}

const DU = new RegExp(
  `(?<![${LETTER}])(Deines|Deinem|Deinen|Deiner|Deine|Dein|Dich|Dir|Du)(?![${LETTER}])`,
  "g",
);

// What can stand in front of a capital that opens a sentence. `\}\s` is here
// because a placeholder's value is text this rule cannot read.
const SENTENCE_OPENS = /(?:[.!?:]\s|\n|[„"(]|\}\s)$/;

function startsASentence(value: string, at: number): boolean {
  return at === 0 || SENTENCE_OPENS.test(value.slice(0, at));
}

/** Every finding over the given catalogs, each led by the check it breaks. */
export function registerFindings(copies: readonly LocaleCopy[]): string[] {
  const entries = catalogEntries(copies);
  return [
    ...entries.flatMap(([source, key, value]) =>
      readByAnOutsider(key)
        ? []
        : [...value.matchAll(FORMAL)].map((hit) => {
            const position = startsASentence(value, hit.index)
              ? "opening a sentence"
              : "mid-sentence";
            return `addresses the reader as Sie or Ihr: ${source} ${key}: "${hit[0]}" ${position} in ${value}`;
          }),
    ),
    ...Object.keys(READ_BY_AN_OUTSIDER)
      .filter((prefix) => !entries.some(([, key]) => key.startsWith(prefix)))
      .map(
        (prefix) =>
          `every exempt prefix names a key the catalogs carry: ${prefix}`,
      ),
    ...entries.flatMap(([source, key, value]) =>
      [...value.matchAll(DU)]
        .filter((hit) => !startsASentence(value, hit.index))
        .map(
          (hit) =>
            `capitalises a du-pronoun mid-sentence: ${source} ${key}: "${hit[0]}" in ${value}`,
        ),
    ),
  ];
}

export const rule: CensusRule = {
  name: "address-register",
  corpus: (tree) =>
    tree.files.filter(
      (file) => file.path === coreCatalog || BUYER_PAGE.test(file.rel),
    ),
  floors: (corpus) =>
    corpus.some((file) => BUYER_PAGE.test(file.rel))
      ? []
      : ["the buyer's page is a file under src/features/deals/buyerroom*.tsx"],
  across: (corpus) => {
    const core = corpus.find((file) => file.path === coreCatalog)?.source;
    if (!core) {
      return [];
    }
    const copies = germanCopies(core);
    const pages = corpus.filter((file) => BUYER_PAGE.test(file.rel));
    return [...registerFindings(copies), ...buyerPageFindings(copies, pages)];
  },
  advice:
    "German copy speaks to its reader as du. Rewrite a formal Sie or Ihr, or " +
    "a du-pronoun capitalised mid-sentence; only the families in " +
    "READ_BY_AN_OUTSIDER (scripts/lib/german-catalogs.ts) keep Sie. A du " +
    "line on the buyer's page moves into a buyer. key, written in Sie.",
};
