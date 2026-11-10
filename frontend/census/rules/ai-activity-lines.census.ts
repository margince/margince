// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The email mutation keys stay split at the call sites. A draft is the AI call
// the rail narrates under ["email-draft", …]; a send is the write the ticker
// narrates under ["email", …]. A screen spelling either twice narrates one
// action under two vocabularies.
//
// src/app/ai-activity-lines.test.ts holds the ticker's tables to the same
// split. A draft mutation renamed back to ["email", …] leaves those tables
// untouched, so the call sites are read here. A mutationKey is a literal inside
// a hook and exported by nothing, so it is read from source.

import type { CensusFile, CensusRule } from "../tree";
import { screenCorpus } from "./screenfiles.census";

const KEYS = ['["email-draft",', '["email",'];

const count = (haystack: string, needle: string) =>
  haystack.split(needle).length - 1;

/** Each email mutation key this screen spells more than once. */
export function doubledKeys(file: CensusFile): string[] {
  return KEYS.filter((key) => count(file.text, key) > 1).map(
    (key) => `${file.rel} spells ${key} …] ${count(file.text, key)} times`,
  );
}

export const rule: CensusRule = {
  name: "ai-activity-lines",
  // Every screen that spells either key, so a new screen that takes one up is
  // read the day it is written.
  corpus: (tree) =>
    screenCorpus(tree).filter((file) =>
      KEYS.some((key) => file.text.includes(key)),
    ),
  visit: doubledKeys,
  advice:
    "A screen spells an email mutation key twice. Keep one draft mutation " +
    'under ["email-draft", …] and one send under ["email", …], so each ' +
    "action is narrated once.",
};
