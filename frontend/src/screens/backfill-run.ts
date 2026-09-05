// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

// The connect-time import, as both of its surfaces need it: the run row, the
// four operations on it, and the window pick that ties them together.

type BackfillStatus = components["schemas"]["BackfillStatus"];
type BackfillPreview = components["schemas"]["BackfillPreview"];
type Provider = components["schemas"]["CaptureConnection"]["provider"];

/** The startable windows. `none` is expressed by never starting a run at all. */
export type ImportWindow =
  components["schemas"]["StartBackfillRequest"]["window"];

/**
 * The window a picker opens on. A multi-year import is worth offering and wrong
 * to default into: it spends the customer's own inference budget on the day they
 * have least reason to trust the estimate.
 */
export const DEFAULT_IMPORT_WINDOW: ImportWindow = "6m";

/** A run that can still move. Every other state is terminal. */
export function isLiveRun(state: BackfillStatus["state"] | undefined): boolean {
  return state === "queued" || state === "running";
}

// The status read is a single indexed row (CAP-PARAM-2), so polling it is
// indistinguishable from a push — and one cadence, so one mailbox is never
// watched at two rates by the two surfaces that watch it.
const POLL_MS = 2500;

/** The one spelling of the run-row cache key: a start or a stop on either
 *  surface invalidates the read on both. */
const statusQueryKey = (provider: Provider) => ["backfill-status", provider];

async function readRun(provider: Provider): Promise<BackfillStatus> {
  const { data, error } = await api.GET("/connectors/{provider}/backfill", {
    params: { path: { provider } },
  });
  if (error) {
    throwProblem(error);
  }
  return data;
}

async function previewRun(
  provider: Provider,
  window: ImportWindow,
): Promise<BackfillPreview> {
  const { data, error } = await api.POST(
    "/connectors/{provider}/backfill/preview",
    { params: { path: { provider } }, body: { window } },
  );
  if (error) {
    throwProblem(error);
  }
  return data;
}

async function startRun(
  provider: Provider,
  window: ImportWindow,
): Promise<BackfillStatus> {
  const { data, error } = await api.POST("/connectors/{provider}/backfill", {
    params: { path: { provider } },
    body: { window },
  });
  if (error) {
    throwProblem(error);
  }
  return data;
}

async function cancelRun(provider: Provider): Promise<BackfillStatus> {
  const { data, error } = await api.DELETE("/connectors/{provider}/backfill", {
    params: { path: { provider } },
  });
  if (error) {
    throwProblem(error);
  }
  return data;
}

/**
 * The run row and the four operations on it, in one spelling.
 *
 * Two surfaces draw a connect-time import: the Settings connections card
 * (backfill.tsx) and the last step of onboarding (onboarding-backread.tsx).
 * They ask the reader different questions and say different things about the
 * answer — that is theirs — but the read, the estimate, the start, the stop and
 * the pick that ties them together are ONE behaviour, and they held two copies
 * of it. The copies had already begun to disagree in the way copies do: each
 * carried its own spelling of the cache key under a comment claiming the key
 * was shared, so an invalidation from one surface reached the other only
 * because the two strings happened to still match.
 *
 * What stays with each caller is what genuinely differs: which refusals it can
 * name, whether an estimate it has not seen may be acted on, and every word on
 * screen.
 */
export function useBackfillRun({
  provider,
  initial,
  previewHeld = false,
}: Readonly<{
  provider: Provider;
  /** The run row already embedded in the `GET /connectors` roster row — seeds
   *  the first render, so a live run shows immediately and never offers to
   *  start a second one. */
  initial?: BackfillStatus;
  /** The caller has taken the setup view off screen for a reason of its own
   *  (Settings' "skip the history import"), so the scope must not be counted
   *  for a form nobody is looking at. */
  previewHeld?: boolean;
}>) {
  const qc = useQueryClient();
  const [window, setWindow] = useState<ImportWindow>(DEFAULT_IMPORT_WINDOW);
  // A run that has stopped — cancelled, failed or finished — is history, not a
  // reason the mailbox can never be imported again. This puts the window pick
  // back in front of the reader; the server still decides whether the pick is
  // allowed (widen-only), exactly as it does the first time.
  const [restarting, setRestarting] = useState(false);
  const [previewedWindow, setPreviewedWindow] = useState<ImportWindow | null>(
    null,
  );

  const status = useQuery({
    queryKey: statusQueryKey(provider),
    queryFn: () => readRun(provider),
    initialData: initial,
    // The embedded snapshot already answered this read once; skip the
    // mount-time revalidation react-query would otherwise run (it treats
    // fresh-but-present data as needing it) and rely on the live poll below, or
    // an explicit invalidate, for a fresher row. Without a seed, fetch on mount.
    staleTime: initial !== undefined ? Number.POSITIVE_INFINITY : 0,
    refetchInterval: (query) =>
      isLiveRun(query.state.data?.state) ? POLL_MS : false,
  });

  const preview = useMutation({
    mutationFn: (pick: ImportWindow) => previewRun(provider, pick),
  });

  const start = useMutation({
    mutationFn: (pick: ImportWindow) => startRun(provider, pick),
    onSuccess: () => {
      // The new run is what the reader watches now, so the pick they started it
      // from stands down with it.
      setRestarting(false);
      return qc.invalidateQueries({ queryKey: statusQueryKey(provider) });
    },
  });

  const cancel = useMutation({
    mutationFn: () => cancelRun(provider),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: statusQueryKey(provider) }),
  });

  // The scope loads itself for whichever window is selected: the first thing a
  // newly-connected reader sees is honest scope, not an empty form. A preview is
  // a read and spends nothing; the spend still waits for the explicit start.
  // Single-flight per window — `previewedWindow` stops it re-firing on unrelated
  // re-renders while still re-counting on a window change.
  const isSetup = !previewHeld && (status.data?.state === "none" || restarting);
  useEffect(() => {
    if (!isSetup || previewedWindow === window || preview.isPending) {
      return;
    }
    setPreviewedWindow(window);
    preview.mutate(window);
  }, [isSetup, window, previewedWindow, preview]);

  return {
    status,
    preview,
    start,
    cancel,
    /** The window the picker is on. */
    window,
    setWindow,
    /** The pick is on screen: no run has ever started, or a stopped one is
     *  being started again. */
    isSetup,
    /**
     * Reopen the pick on a stopped run.
     *
     * It opens on the window that run used, because the server refuses a
     * narrower one (widen-only) and a picker that opens on a refusal wastes the
     * reader's first press. The scope is re-counted rather than re-shown: the
     * estimate that consented to the run which just ended was measured before
     * that run captured anything.
     */
    restart: (run: BackfillStatus) => {
      setWindow(run.window ?? DEFAULT_IMPORT_WINDOW);
      setPreviewedWindow(null);
      setRestarting(true);
    },
  };
}
