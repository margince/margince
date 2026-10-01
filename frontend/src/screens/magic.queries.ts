// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

export type MagicReceipt = components["schemas"]["MagicReceipt"];
export type MagicLine = components["schemas"]["MagicLine"];
export type MagicNotShown = components["schemas"]["MagicNotShown"];
// Derived from the line rather than spelled again: a lane renamed in crm.yaml
// is a compile error here instead of a heading over rows that never arrive.
export type MagicLane = MagicLine["lane"];

export const magicKey = ["magic"] as const;

/**
 * How far back the page looks. "brief" leaves `since` to the server, which
 * resolves it to the reader's own last brief cutoff — the window they have not
 * already read. The other two ask for a fixed stretch, for the reader who was
 * away or wants to see what an import set off; the server still caps any
 * request at 30 days.
 */
export const MAGIC_WINDOWS = ["brief", "week", "month"] as const;
export type MagicWindow = (typeof MAGIC_WINDOWS)[number];

const WINDOW_DAYS: Readonly<Record<Exclude<MagicWindow, "brief">, number>> = {
  week: 7,
  month: 30,
};

const DAY_MS = 24 * 60 * 60 * 1000;

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

/** The `since` a window asks for, or undefined to leave it to the server. */
export function sinceFor(span: MagicWindow, now: Date): string | undefined {
  if (span === "brief") {
    return undefined;
  }
  return new Date(now.getTime() - WINDOW_DAYS[span] * DAY_MS).toISOString();
}

/**
 * What the machinery did in the chosen window.
 *
 * No `enabled` gate: every line is about the reader's own records, scoped by
 * the same row predicates their record pages use, and the server refuses a
 * caller with no human behind it — which is the only refusal this read has.
 *
 * The key carries the WINDOW, not the instant: the instant moves every
 * render, and a key that moves with it would refetch forever.
 */
export function useMagic(span: MagicWindow = "brief") {
  return useQuery({
    queryKey: [...magicKey, span],
    queryFn: async (): Promise<MagicReceipt> => {
      const since = sinceFor(span, new Date());
      const { data, error } = await api.GET("/magic", {
        params: {
          query: { limit: MAGIC_PAGE_LINES, ...(since ? { since } : {}) },
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}
