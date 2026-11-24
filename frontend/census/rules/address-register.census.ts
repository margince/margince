// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// German copy speaks to its reader as du, and three checks hold that.
//
// 1. No value addresses the reader formally. Capitalised Sie / Ihr… is that
//    address, and only the key families an outside reader opens are exempt.
//    "Sie" opening a sentence as she or they is rephrased, never waived.
// 2. A du-pronoun is capitalised only where a sentence begins. Mid-sentence it
//    is the formal register wearing the informal word.
// 3. Those outsider families never say du, so their surfaces hold one address.
// 4. The buyer's page speaks Sie throughout, so no key its own files name from
//    outside the outsider families carries a du-pronoun.
//
// The mechanics of German copy are the copy-style-de rule's; both read the
// catalogs it derives.

import { dirname, resolve } from "node:path";
import {
  LETTER,
  READ_BY_AN_OUTSIDER,
  readByAnOutsider,
} from "../../scripts/lib/german-catalogs";
import {
  catalogEntries,
  type LocaleCopy,
} from "../../scripts/lib/locale-catalogs";
import type { CensusFile, CensusRule, CensusTree } from "../tree";
import { coreCatalog, germanCopies } from "./copy-style-de.census";

// Longest first: the alternation is what gets reported, and "Ihr" would match
// the head of "Ihrem" and name the wrong word in the finding.
const FORMAL = new RegExp(
  `(?<![${LETTER}])(Ihnen|Ihres|Ihrem|Ihren|Ihrer|Ihre|Ihr|Sie)(?![${LETTER}])`,
  "g",
);

// The buyer's page: its entry file and every file under src/features/deals it
// imports, followed through each import. Stories and tests are not imported.
const BUYER_ENTRY = "src/features/deals/buyerroom.tsx";
const BUYER_FEATURE = "src/features/deals/";
const RELATIVE_IMPORT = /from\s+"(\.[^"]+)"/g;
const QUOTED = /"([a-z][A-Za-z0-9]*\.[A-Za-z0-9_.]+)"/g;

/** The files of the buyer's page, entry first. */
export function buyerPageFiles(tree: CensusTree): CensusFile[] {
  const entry = tree.files.find((file) => file.rel === BUYER_ENTRY);
  const reached = entry ? [entry] : [];
  for (const file of reached) {
    for (const [, specifier] of file.text.matchAll(RELATIVE_IMPORT)) {
      const base = resolve(dirname(file.path), specifier);
      const next = [".tsx", ".ts"]
        .map((extension) => tree.at(base + extension))
        .find((found) => found !== undefined);
      if (next?.rel.startsWith(BUYER_FEATURE) && !reached.includes(next)) {
        reached.push(next);
      }
    }
  }
  return reached;
}

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
      .filter(([, , value]) => value.match(ANY_DU) !== null)
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

const ANY_DU = new RegExp(DU.source, "gi");

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
      readByAnOutsider(key)
        ? [...value.matchAll(ANY_DU)].map(
            (hit) =>
              `addresses an outside reader as du: ${source} ${key}: "${hit[0]}" in ${value}`,
          )
        : [],
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
  corpus: (tree) => tree.files.filter((file) => file.path === coreCatalog),
  floors: (_corpus, tree) =>
    buyerPageFiles(tree).length > 1
      ? []
      : [`the buyer's page is ${BUYER_ENTRY} and the files it imports`],
  across: (corpus, tree) => {
    const core = corpus[0]?.source;
    if (!core) {
      return [];
    }
    const copies = germanCopies(core);
    return [
      ...registerFindings(copies),
      ...buyerPageFindings(copies, buyerPageFiles(tree)),
    ];
  },
  advice:
    "German copy speaks to its reader as du. Rewrite a formal Sie or Ihr, or " +
    "a du-pronoun capitalised mid-sentence. Only the families in " +
    "READ_BY_AN_OUTSIDER (scripts/lib/german-catalogs.ts) keep Sie, and " +
    "they never say du. A du line on the buyer's page moves into a buyer. " +
    "key, written in Sie.",
};
