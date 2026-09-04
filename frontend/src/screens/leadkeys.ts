// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { QueryKey } from "@tanstack/react-query";

// Which cached reads a write to ONE lead makes stale, and the keys those reads
// are filed under — spelled here rather than at each site, the way
// `activitykeys.ts` already does it for a record's timeline.
//
// It exists because the pair is not one key. The board and the list read
// `["leads", query]`; the detail page reads the SIBLING `["lead", id]`, and
// prefix invalidation does not walk sideways. A mutation naming only the list
// leaves an open detail page showing the value the reader just changed —
// which is what a board drag and a bulk assign both did.
//
// The audit row is part of the same write. Every lead mutation the server
// accepts writes one, so `["record-history", "lead", id]` is stale whenever
// `["lead", id]` is. Three of the eight write sites remembered that and five
// did not, which is the shape a hand-spelled key set always ends up in.

/** The list and the board. Their read key is `["leads", query]`; this prefix reaches both. */
export const LEAD_LIST_KEY: QueryKey = ["leads"];

/**
 * How many leads each status holds, from the leads-by-status report.
 *
 * Under LEAD_LIST_KEY's prefix on purpose: the figures and the cards go stale
 * together. A qualified lead has to leave the open column and arrive in the
 * terminal count in the same beat, and the ONE invalidation every lead write
 * already fires is what makes that true.
 */
export const LEAD_STATUS_COUNTS_KEY: QueryKey = [
  ...LEAD_LIST_KEY,
  "by-status-counts",
];

/**
 * The archived leads in ONE terminal column, read only while it is open.
 *
 * Under the same prefix, for the same reason: qualifying a lead adds a row to
 * this list, and a column left holding its old page would be missing the lead
 * the reader just put there.
 */
export function leadTerminalKey(status: string): QueryKey {
  return [...LEAD_LIST_KEY, "terminal", status];
}

/** The detail page. Its children — the score and the manual signals below — are prefix-reached from here. */
export function leadKey(id: string): QueryKey {
  return ["lead", id];
}

/** The scoring breakdown the overview pane draws. */
export function leadScoreKey(id: string): QueryKey {
  return [...leadKey(id), "score"];
}

/** The signals a human added by hand, which the signals pane reads and writes. */
export function leadManualSignalsKey(id: string): QueryKey {
  return [...leadKey(id), "manual-signals"];
}

// Deliberately NOT part of the write set below, and this is the one entry that
// needs saying out loud. The promote preview asks what qualifying this lead
// would do to the workspace as it stands the moment the dialog opens, so it is
// declared `staleTime: 0` and `enabled: open` — it refetches on every open.
// Invalidating it here would change nothing about that: an inactive query is
// refetched when it next mounts either way.
//
// What it does NOT do is forget the previous answer. React Query's default
// `gcTime` keeps it for five minutes, so a reopen inside that window paints the
// LAST open's preview — a "create" that may be a merge by now — for as long as
// the refetch takes. That is a real thing a reader can see, and it is not this
// module's to fix: invalidating a cached-but-inactive query does not purge its
// data either. Saying it here rather than claiming the query "caches nothing"
// keeps the next reader from trusting a sentence that is false.
export function leadPromotePreviewKey(id: string): QueryKey {
  return ["lead-promote-preview", id];
}

/**
 * Every cached read one lead's write makes stale: the list it appears in, its
 * own detail page, and the history of what changed on it.
 *
 * Pass the id of the lead that was WRITTEN. A bulk run writes several and owes
 * this for each of them — the row whose owner just changed is stale on its own
 * page whether it was written alone or as one of forty.
 */
export function leadWriteKeys(id: string): QueryKey[] {
  return [LEAD_LIST_KEY, leadKey(id), ["record-history", "lead", id]];
}
