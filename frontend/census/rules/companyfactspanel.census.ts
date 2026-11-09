// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The company-facts query key is declared once, in companyfactspanel.tsx, as
// factsKey. Several places read or settle it. A copy written out by hand would
// miss a rename. Then a panel would keep showing "Nothing on file yet" beside
// facts a finished read has staged.

import { join } from "node:path";
import { type CensusFile, type CensusRule, srcDir } from "../tree";

// Assembled rather than written, so this file and its test are no hit.
export const KEY_LITERAL = `"${["company", "facts"].join("-")}"`;

const OWNER = join(srcDir, "features", "companies", "companyfactspanel.tsx");

// Tests and stories included: a census that skips a folder passes over the
// offender in it.
const inCorpus = (file: CensusFile) =>
  file.area === "src" && /\.tsx?$/.test(file.path) && file.path !== OWNER;

export const rule: CensusRule = {
  name: "companyfactspanel",
  corpus: (tree) => tree.files.filter(inCorpus),
  visit: (file) =>
    file.text.includes(KEY_LITERAL)
      ? [`${file.rel}: spells the company-facts key itself`]
      : [],
  advice:
    "These spell the facts key themselves instead of calling factsKey(), " +
    "so a rename would reach some readers and not others.",
};
