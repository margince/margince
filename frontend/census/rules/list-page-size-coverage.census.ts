// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One page size. `ListQuery.perPage` is the size the reader picks in the table
// footer, and every fetcher's `limit` derives from it through `listFetchLimit`,
// a whole number of rendered pages per read.
//
// A literal limit renders a page size the server never returned: the footer
// offers 100, the response holds 50. A limit of `query.perPage` itself leaves
// the pager unable to offer a second number until the reader asks for it.
//
// The screens come from the directory, so a new list screen is covered the
// day it is written.

import { basename } from "node:path";
import type { CensusFile, CensusRule } from "../tree";
import { screenCorpus } from "./screenfiles.census";

// Calls the wrapper rather than declaring it: listquery.tsx owns no fetcher.
const readsAList = (file: CensusFile) =>
  /\buseListQuery</.test(file.text) &&
  !/export function useListQuery/.test(file.text);

/**
 * Whether the list fetcher's own `limit` derives from the query. A typeahead on
 * the same screen caps itself, so the fetcher is found by the `cursor` beside
 * its limit, which no other read carries.
 */
export function pageSizeFindings(file: CensusFile): string[] {
  const fetcher = file.text.match(
    /cursor: cursor \|\| undefined,\s*\n\s*limit: ([^,\n]+),/,
  );
  if (!fetcher) {
    return [
      `${file.rel} has no list fetcher shaped like the shared one; if the shape changed, change this check with it`,
    ];
  }
  return fetcher[1] === "listFetchLimit(query.perPage)"
    ? []
    : [`${file.rel} sends the page limit ${fetcher[1]}`];
}

export const rule: CensusRule = {
  name: "list-page-size-coverage",
  corpus: (tree) => screenCorpus(tree).filter(readsAList),
  floors: (corpus) => {
    const names = new Set(corpus.map((file) => basename(file.path)));
    return [
      ...["contacts.tsx", "companies.list.tsx"]
        .filter((name) => !names.has(name))
        .map((name) => `${name} was not found among the list screens`),
      ...(corpus.length >= 6 ? [] : ["fewer than 6 list screens were found"]),
    ];
  },
  visit: pageSizeFindings,
  advice:
    "A list screen sends a fixed page limit, so the footer's page size and " +
    "the server's can disagree. Send limit: listFetchLimit(query.perPage).",
};
