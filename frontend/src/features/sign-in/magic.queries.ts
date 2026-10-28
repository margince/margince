// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { queryOptions, useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { unwrap } from "../../lib/problem";
import { magicKey } from "../../lib/workkeys";

export type MagicReceipt = components["schemas"]["MagicReceipt"];
export type MagicLine = components["schemas"]["MagicLine"];
export type MagicNotShown = components["schemas"]["MagicNotShown"];
// Derived from the line rather than spelled again: a lane renamed in crm.yaml
// is a compile error here instead of a heading over rows that never arrive.
export type MagicLane = MagicLine["lane"];

export { magicKey } from "../../lib/workkeys";

// The most lines a lane's page may hold, asked for rather than left to the
// server's default so the client knows its own bound: the receipt carries no
// "more" flag, so a lane that FILLS its page may have more beyond it, and every
// sum taken over that lane is then only a floor.
export const MAGIC_PAGE_LINES = 100;

// Which lanes the page bounds. Watching is every standing condition there is,
// read without the bound, so however many it holds it holds them all.
const PAGE_BOUND: Readonly<Record<MagicLane, boolean>> = {
  done: true,
  needs_you: true,
  could_not_complete: true,
  watching: false,
};

/** Whether a lane filled its page, and so may hold more than it shows. */
export function fillsPage(
  lane: MagicLane,
  rows: readonly MagicLine[],
): boolean {
  return PAGE_BOUND[lane] && rows.length >= MAGIC_PAGE_LINES;
}

/**
 * What the machinery did since the reader's last brief: `since` is left to the
 * server, which resolves it to the reader's own last brief cutoff.
 *
 * No `enabled` gate: every line is about the reader's own records, scoped by
 * the same row predicates their record pages use, and the server refuses a
 * caller with no human behind it — which is the only refusal this read has.
 */
export function useMagic() {
  return useQuery(magicOptions);
}

/** The read useMagic makes, which Home's prefetch also starts. */
export const magicOptions = queryOptions({
  queryKey: magicKey,
  queryFn: async (): Promise<MagicReceipt> =>
    unwrap(
      await api.GET("/magic", {
        params: { query: { limit: MAGIC_PAGE_LINES } },
      }),
    ),
});
