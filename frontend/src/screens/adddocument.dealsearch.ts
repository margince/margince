// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { FIRST_PAGE } from "../api/client";
import type { components } from "../api/schema";
import type { RecordPickerCandidate } from "../design-system/recordpicker";
import { foldForMatch } from "../format/collate";

type DealPage = components["schemas"]["DealListResponse"];

// HOW THE DEAL SEARCH WORKS, AND WHERE IT STOPS.
//
// `GET /deals` is cursor-paginated and takes no text query — the contract
// offers a cursor, a limit, a sort and a set of id filters, and nothing
// textual. So the words the reader types are matched HERE, over pages this
// dialog walks, and a client-side match has to stop somewhere or one settled
// keystroke walks every deal an old account ever had.
//
// The bound is pages, not results: DEAL_SEARCH_PAGES pages of the contract's
// maximum page size, in the list endpoint's own default order, which is
// newest-created first. What the search therefore covers is this account's
// DEAL_SEARCH_REACH newest deals, and what it cannot reach is anything older —
// which the picker STATES, under the field, before the reader goes looking. An
// unfound deal and a deal that does not exist read identically otherwise, and
// that silence is the whole of what issue 1536 was about.
export const DEAL_PAGE_SIZE = 200;
const DEAL_SEARCH_PAGES = 10;
export const DEAL_SEARCH_REACH = DEAL_PAGE_SIZE * DEAL_SEARCH_PAGES;

// How many matches are worth offering at once. Past this the walk stops: a
// list of a hundred pickable buttons is not a pick, and the reader has a
// cheaper way to shorten it, which is one more word.
export const DEAL_MATCH_LIMIT = 25;

// How long a walked page is reused. The reader re-runs the whole walk on each
// word they change, so pages are cached under their cursor; a minute outlasts
// a dialog and is far shorter than the age of the deals such a walk reaches.
export const DEAL_PAGE_FRESH_MS = 60_000;

/**
 * Walk the account's deals, newest first, keeping the ones whose name contains
 * what the reader typed.
 *
 * `fetchPage` is injected rather than called directly so the walk reads pages
 * through the caller's cache: the second search over one account re-reads what
 * the first already fetched instead of spending the whole page budget again.
 */
export async function walkAccountDeals(
  fetchPage: (cursor: string | null) => Promise<DealPage>,
  needle: string,
): Promise<RecordPickerCandidate[]> {
  const matches: RecordPickerCandidate[] = [];
  let cursor = FIRST_PAGE;
  for (let page = 0; page < DEAL_SEARCH_PAGES; page += 1) {
    const answered = await fetchPage(cursor);
    for (const deal of answered.data) {
      if (foldForMatch(deal.name).includes(needle)) {
        matches.push({ id: deal.id, name: deal.name });
      }
    }
    if (matches.length >= DEAL_MATCH_LIMIT) {
      return matches.slice(0, DEAL_MATCH_LIMIT);
    }
    // The CURSOR is what the walk can continue with, and `has_more` without one
    // is a cut list nothing can read the rest of — so both ends of the walk end
    // it here rather than looping on a cursor that will not move.
    cursor = answered.page.next_cursor ?? null;
    if (!cursor) {
      return matches;
    }
  }
  return matches;
}
