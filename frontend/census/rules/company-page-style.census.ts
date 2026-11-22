// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The company page writes a date one way and money one way. Then two panels
// never show one value in two spellings. A date reads "16 Nov 2026", with the
// time after it where a row needs one. Money reads as on the deal page: €20,000,
// cents only when there are some. The compact form stays for tiles. History
// belongs to the records feature, so its files are in the corpus too.

import { basename } from "node:path";
import ts from "typescript";
import { productionModules } from "../imports";
import type { CensusFile, CensusRule } from "../tree";

/** The formatter the page uses in place of each other spelling. */
export const PAGE_SPELLING: ReadonlyMap<string, string> = new Map([
  ["formatDate", "formatDateAbbrev"],
  ["formatDateTime", "formatDateTimeAbbrev"],
  ["formatMoney", "formatMoneyWhole"],
  ["formatMoneyOrAbsent", "formatMoneyOrWord with formatMoneyWhole"],
]);

const HISTORY_ROWS = "src/features/records/historyentries.tsx";

const onThePage = (file: CensusFile) =>
  file.rel.startsWith("src/features/companies/") ||
  basename(file.path).startsWith("history");

function otherSpellings(source: ts.SourceFile): Set<string> {
  const found = new Set<string>();
  const walk = (node: ts.Node) => {
    if (ts.isIdentifier(node) && PAGE_SPELLING.has(node.text)) {
      found.add(node.text);
    }
    ts.forEachChild(node, walk);
  };
  walk(source);
  return found;
}

export const rule: CensusRule = {
  name: "company-page-style",
  corpus: (tree) =>
    productionModules(tree.files, [
      "features/companies",
      "features/records",
    ]).filter(onThePage),
  floors: (corpus) =>
    corpus.some((file) => file.rel === HISTORY_ROWS)
      ? []
      : [`the census did not reach ${HISTORY_ROWS}`],
  visit: (file) =>
    file.source
      ? [...otherSpellings(file.source)].map(
          (name) =>
            `${file.rel}: ${name} (use ${PAGE_SPELLING.get(name) ?? ""})`,
        )
      : [],
  advice:
    "These write a date or an amount in a second style on the company " +
    "page. Use the page's spelling, so one value reads the same in every panel.",
};
