// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The last Undo of many deals, kept for this reader across a reload, and its
// answers read again against the change history.

import { useSyncExternalStore } from "react";
import type { Receipt } from "./handled";
import {
  type BundleUndoRun,
  type MarginceChangeBundle,
  type MarginceChangeSummary,
  stillPending,
  type UndoMarginceChangeResult,
} from "./marginchanges";
import { useViewerId } from "./session";
import {
  readStored,
  readStoredJson,
  removeStored,
  STORAGE_KEYS,
  watchStored,
  writeStored,
} from "./storage";

const KEY = STORAGE_KEYS.lastBulkUndo;

const watch = (changed: () => void) => watchStored(KEY, changed);
const snapshot = () => readStored(KEY);

/** A kept run, with the name of every deal it answered for. */
export type RememberedRun = Readonly<{
  run: BundleUndoRun;
  names: Readonly<Record<string, string>>;
}>;

/**
 * The run this browser kept for the signed-in seat, or null. A run kept for
 * another seat reads as none, so a session that lapsed hands nothing to the
 * next one. A run's error is not kept.
 */
export function useRememberedUndoRun(): RememberedRun | null {
  const seat = useViewerId();
  // The stored text is the snapshot, so a render reads one parse per write.
  const raw = useSyncExternalStore(watch, snapshot, snapshot);
  return raw === null ? null : rememberedUndoRun(seat);
}

/** The run kept for `seat` as it is now, outside a render. */
export function rememberedUndoRun(
  seat: string | undefined,
): RememberedRun | null {
  const kept = readStoredJson(KEY, parseRemembered);
  return kept && seat !== undefined && kept.seat === seat ? kept : null;
}

/** Keeps the run for `seat`, or forgets it for null. With no seat known yet
 *  nothing is written. */
export function rememberUndoRun(
  seat: string | undefined,
  remembered: RememberedRun | null,
): void {
  if (seat === undefined) return;
  if (remembered === null) {
    removeStored(KEY);
    return;
  }
  const { run, names } = remembered;
  writeStored(
    KEY,
    JSON.stringify({
      seat,
      results: run.results,
      unsent: run.unsent,
      settled: run.settled ?? [],
      names,
    }),
  );
}

/**
 * The run with each deal it could not send read against what the server says
 * now. A deal whose changes the change history shows reversed is undone. A
 * deal the pending read no longer lists is settled, by this run or otherwise.
 * Only a deal that still waits stays unsent.
 */
export function settledRun(
  run: BundleUndoRun,
  receipts: readonly Receipt[] | undefined,
  pending: MarginceChangeSummary | undefined,
): BundleUndoRun {
  const reversed = new Set(
    (receipts ?? [])
      .filter((receipt) => receipt.undo?.reversed || receipt.review?.reversed)
      .flatMap((receipt) => receipt.audit_log_id ?? []),
  );
  const undone = run.unsent.filter((bundle) =>
    bundle.audit_log_ids.every((id) => reversed.has(id)),
  );
  const left = run.unsent.filter(
    (bundle) =>
      !undone.includes(bundle) &&
      pending !== undefined &&
      stillPending([bundle], pending).length === 0,
  );
  if (undone.length === 0 && left.length === 0) return run;
  return {
    ...run,
    results: [
      ...run.results,
      ...undone.map((bundle) => ({
        deal_id: bundle.deal_id,
        outcome: "undone" as const,
        undone: [...bundle.kinds],
        kept: [],
      })),
    ],
    settled: [...(run.settled ?? []), ...left],
    unsent: run.unsent.filter(
      (bundle) => !undone.includes(bundle) && !left.includes(bundle),
    ),
  };
}

function parseRemembered(
  value: unknown,
): (RememberedRun & { seat: string }) | null {
  if (typeof value !== "object" || value === null) return null;
  if (!("seat" in value) || typeof value.seat !== "string") return null;
  const results = "results" in value ? listOf(value.results, isResult) : null;
  const unsent = "unsent" in value ? listOf(value.unsent, isBundle) : null;
  const settled = "settled" in value ? listOf(value.settled, isBundle) : [];
  if (!results || !unsent || !settled) return null;
  const names = "names" in value ? namesOf(value.names) : {};
  return { seat: value.seat, run: { results, unsent, settled }, names };
}

// The list when every entry has the shape, else null.
function listOf<T>(value: unknown, is: (entry: unknown) => entry is T) {
  return Array.isArray(value) && value.every(is) ? value : null;
}

function namesOf(value: unknown): Record<string, string> {
  const names: Record<string, string> = {};
  if (typeof value !== "object" || value === null) return names;
  for (const [deal, name] of Object.entries(value)) {
    if (typeof name === "string") names[deal] = name;
  }
  return names;
}

function isResult(value: unknown): value is UndoMarginceChangeResult {
  return (
    typeof value === "object" &&
    value !== null &&
    "deal_id" in value &&
    typeof value.deal_id === "string" &&
    "outcome" in value &&
    typeof value.outcome === "string" &&
    "undone" in value &&
    Array.isArray(value.undone) &&
    "kept" in value &&
    Array.isArray(value.kept)
  );
}

function isBundle(value: unknown): value is MarginceChangeBundle {
  return (
    typeof value === "object" &&
    value !== null &&
    "deal_id" in value &&
    typeof value.deal_id === "string" &&
    "version" in value &&
    typeof value.version === "number" &&
    "audit_log_ids" in value &&
    Array.isArray(value.audit_log_ids) &&
    "kinds" in value &&
    Array.isArray(value.kinds)
  );
}
