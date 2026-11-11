// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useMemo, useSyncExternalStore } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { unwrap } from "./problem";
import { isRecord } from "./problembody";
import { readStored, STORAGE_KEYS, watchStored, writeStored } from "./storage";

type UndoItem = components["schemas"]["UndoMarginceChangeItem"];
type UndoRun = components["schemas"]["MarginceUndoRun"];
export type UndoManifest = { batches: { id: string; items: UndoItem[] }[] };
export class BulkUndoStorageError extends Error {}
const key = STORAGE_KEYS.lastBulkUndo;
const subscribe = (changed: () => void) => watchStored(key, changed);
const snapshot = () => readStored(key);

function undoItem(value: unknown): UndoItem | null {
  if (!isRecord(value)) return null;
  const { deal_id, version, audit_log_ids, archive_confirmed_for } = value;
  if (
    typeof deal_id !== "string" ||
    !Number.isSafeInteger(version) ||
    typeof version !== "number" ||
    !Array.isArray(audit_log_ids) ||
    !audit_log_ids.every((id): id is string => typeof id === "string") ||
    (archive_confirmed_for !== undefined &&
      typeof archive_confirmed_for !== "string")
  )
    return null;
  return { deal_id, version, audit_log_ids, archive_confirmed_for };
}

function decodeManifest(raw: string | null): UndoManifest | null {
  if (raw === null) return null;
  let value: unknown;
  try {
    value = JSON.parse(raw);
  } catch {
    return null;
  }
  if (!isRecord(value) || !Array.isArray(value.batches)) return null;
  const batches: UndoManifest["batches"] = [];
  for (const batch of value.batches) {
    if (
      !isRecord(batch) ||
      typeof batch.id !== "string" ||
      !Array.isArray(batch.items)
    )
      return null;
    const items = batch.items.map(undoItem);
    if (!items.every((item): item is UndoItem => item !== null)) return null;
    batches.push({ id: batch.id, items });
  }
  return batches.length > 0 ? { batches } : null;
}

export function keepUndoManifest(
  items: UndoItem[],
  previous: UndoManifest | null = decodeManifest(readStored(key)),
): UndoManifest {
  const batches: UndoManifest["batches"] = [];
  for (let at = 0; at < items.length; at += 200) {
    batches.push({ id: crypto.randomUUID(), items: items.slice(at, at + 200) });
  }
  const manifest = { batches };
  const encoded = JSON.stringify({
    batches: [...(previous?.batches ?? []), ...batches],
  });
  writeStored(key, encoded);
  if (readStored(key) !== encoded)
    throw new BulkUndoStorageError(
      "Bulk Undo could not be saved. Allow site storage and try again.",
    );
  return manifest;
}

export async function beginUndoManifest(
  items: UndoItem[],
): Promise<UndoManifest> {
  const previous = decodeManifest(readStored(key));
  if (!previous) return keepUndoManifest(items, null);
  const saved = await readUndoRuns(previous);
  const complete = new Set(
    saved.filter((run) => run.complete).map((run) => run.run_id),
  );
  return keepUndoManifest(items, {
    batches: previous.batches.filter((batch) => !complete.has(batch.id)),
  });
}

export function useUndoManifest() {
  const raw = useSyncExternalStore(subscribe, snapshot, () => null);
  return useMemo(() => decodeManifest(raw), [raw]);
}

export async function readUndoRuns(manifest: UndoManifest): Promise<UndoRun[]> {
  const runs: UndoRun[] = [];
  for (let at = 0; at < manifest.batches.length; at += 50) {
    const answer = unwrap(
      await api.POST("/deal-corrections/undo/results", {
        body: {
          run_ids: manifest.batches.slice(at, at + 50).map((batch) => batch.id),
        },
      }),
    );
    runs.push(...answer.runs);
  }
  return runs;
}

export function useSavedUndoRuns() {
  const manifest = useUndoManifest();
  return {
    manifest,
    ...useQuery({
      queryKey: ["worklist", "bulk-undo-results", manifest],
      enabled: manifest !== null,
      queryFn: () => (manifest ? readUndoRuns(manifest) : Promise.resolve([])),
      staleTime: 0,
    }),
  };
}

export function summarizeUndoRuns(
  manifest: UndoManifest,
  saved: readonly UndoRun[],
) {
  const completed = new Set(
    saved.filter((run) => run.complete).map((run) => run.run_id),
  );
  const results = saved.flatMap((run) => run.results);
  const answered = new Set(results.map((result) => result.deal_id));
  return {
    results,
    unanswered: manifest.batches
      .filter((batch) => !completed.has(batch.id))
      .flatMap((batch) =>
        batch.items.filter((item) => !answered.has(item.deal_id)),
      ),
  };
}

export async function resumeUndoRuns(manifest: UndoManifest) {
  const saved = await readUndoRuns(manifest);
  const completed = new Map(
    saved.filter((run) => run.complete).map((run) => [run.run_id, run]),
  );
  const results = saved.flatMap((run) => run.results);
  for (let at = 0; at < manifest.batches.length; at++) {
    const batch = manifest.batches[at];
    if (completed.has(batch.id)) continue;
    try {
      const answer = unwrap(
        await api.POST("/deal-corrections/undo", {
          body: { run_id: batch.id, items: batch.items },
        }),
      );
      const earlier = saved.find((run) => run.run_id === batch.id);
      const replaced = new Set(
        earlier?.results.map((result) => result.deal_id),
      );
      const kept = results.filter((result) => !replaced.has(result.deal_id));
      results.splice(0, results.length, ...kept, ...answer.results);
    } catch (error) {
      try {
        const recovered = summarizeUndoRuns(
          manifest,
          await readUndoRuns(manifest),
        );
        return {
          ...recovered,
          error: recovered.unanswered.length > 0 ? error : undefined,
        };
      } catch (recoveryError) {
        return {
          results,
          unanswered: manifest.batches
            .slice(at)
            .flatMap((pending) => pending.items),
          error: new AggregateError(
            [error, recoveryError],
            "Undo delivery and result recovery both failed",
          ),
        };
      }
    }
  }
  return { results, unanswered: [] };
}
