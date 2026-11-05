// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Which of the five things a list read can be, for the Worklist's own panels.
//
// One function because two panels ask it — the team's exceptions and the
// receipts of what was handled — and because the answer they were each
// computing was wrong in the same way: the state came off the query's flags
// alone, so a read that answered with an EMPTY list resolved to `ready` and
// the panel drew a `DataTable`'s header row over no rows. A header row and a
// silence is the one thing a reader cannot act on: it says neither "nothing
// was done" nor "this could not be read", and the two are opposite news.
//
// `sectionState` in the design system answers a neighbouring question and not
// this one: it classifies a section of a COMPOSITE read, keyed on a
// `sections_omitted` list, and it never returns `failed`. These endpoints are
// standalone reads with no withholding view and a refusal worth retrying.

import type { SectionState } from "../design-system/surfacestate";

/**
 * What this function can actually answer.
 *
 * Stated in the signature rather than left as the full vocabulary, for the
 * reason `DerivedSectionState` states it: a return type wider than the
 * behaviour pushes a caller into handling states that cannot arrive. The four
 * that are not here need something only a caller knows — an as-of, a
 * remainder, a mode limitation, a grant.
 */
export type ListReadState = Extract<
  SectionState,
  "loading" | "failed" | "unavailable" | "empty" | "ready"
>;

/**
 * Classify one list read.
 *
 * `rows` is the list off the payload, and its ABSENCE is not its emptiness.
 * Both endpoints declare the list required, so a response without one is
 * version skew rather than a state the server means — and `empty` is the only
 * state allowed to say there is none. Reading an unparseable answer as empty
 * would report a clear team, or a day nothing was done on, over a response
 * nobody could read: the one direction these two surfaces must not fail in.
 *
 * The read is taken as an object rather than as two booleans so a call site
 * cannot pass them the wrong way round — the pair would still typecheck, and
 * `failed` swapped with `loading` is a panel that says a refusal is on its way.
 */
export function listReadState(
  read: Readonly<{ isPending: boolean; isError: boolean }>,
  rows: readonly unknown[] | undefined,
): ListReadState {
  if (read.isPending) {
    return "loading";
  }
  if (read.isError) {
    return "failed";
  }
  if (rows === undefined) {
    return "unavailable";
  }
  return rows.length === 0 ? "empty" : "ready";
}
