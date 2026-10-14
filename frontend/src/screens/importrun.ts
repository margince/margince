// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  skipToken,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { api } from "../api/client";
import type { Locale } from "../i18n";
import { unwrap } from "../lib/problem";
import type { ImportRun } from "./importtypes";

// Every read of the import surface sits under this key. One invalidation after
// a write then refreshes the run, its report and the recent list together.
export const IMPORTS_KEY = "imports";

// The files an import takes, their columns and their order, in the reader's
// language. The template preview, the guide and every screen that orders
// types read it, so none of them keeps a list of its own.
export function useImportFormat(lang: Locale) {
  return useQuery({
    queryKey: [IMPORTS_KEY, "format", lang],
    queryFn: async () =>
      unwrap(await api.GET("/imports/format", { params: { query: { lang } } })),
    staleTime: 60_000,
  });
}

// A run writing or reversing records. The commit and the undo both run
// detached, so only a read tells the screen when they finish.
function writing(status: ImportRun["status"] | undefined): boolean {
  return status === "running" || status === "undoing";
}

// A run still moving under the server's hand, so its reads are polled. A test
// another tab started is moving too, though it writes no record.
export function moving(status: ImportRun["status"] | undefined): boolean {
  return status === "validating" || writing(status);
}

// A draft and a cancelled run wrote nothing, and no screen offers an action on
// them beyond starting again.
export function wroteNothing(status: ImportRun["status"]): boolean {
  return status === "pending" || status === "cancelled";
}

// A run takes minutes and the server computes its rate and ETA at read, so a
// faster poll buys no precision.
export const IMPORT_POLL_MS = 5000;

// The newest runs first, as the server lists them. Polled while one of them is
// still moving, so the list shows the end of a run the reader walked away from.
// A view asks for history or for drafts alone: a draft or a cancelled test must
// not push a finished import out of the one page of history.
export function useRecentImports(view?: "history" | "drafts") {
  return useQuery({
    queryKey: [IMPORTS_KEY, "recent", view ?? "all"],
    queryFn: async () =>
      unwrap(
        await api.GET("/imports", { params: { query: { limit: 20, view } } }),
      ).data,
    refetchInterval: (query) =>
      query.state.data?.some((one) => moving(one.status))
        ? IMPORT_POLL_MS
        : false,
  });
}

// One run, read again every IMPORT_POLL_MS while it is moving.
// It is also read again when the reader returns to the tab, because a run they
// left has most likely moved.
export function useImportRun(id: string | null) {
  return useQuery({
    queryKey: [IMPORTS_KEY, "run", id],
    queryFn:
      id === null
        ? skipToken
        : async () =>
            unwrap(
              await api.GET("/imports/{id}", { params: { path: { id } } }),
            ),
    refetchInterval: (query) =>
      moving(query.state.data?.status) ? IMPORT_POLL_MS : false,
    refetchOnWindowFocus: "always",
  });
}

// Starts the reversal of one run. It answers at once with the run `undoing`;
// the reads above follow it to `undone`.
export function useUndoImport() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) =>
      unwrap(
        await api.POST("/imports/{id}/undo", { params: { path: { id } } }),
      ),
    onSettled: () => queryClient.invalidateQueries({ queryKey: [IMPORTS_KEY] }),
  });
}

// Stops a validating run's test, which returns to its draft with its choices
// and file kept, or cancels a draft. The reads above follow it there.
export function useStopImportTest() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) =>
      unwrap(
        await api.POST("/imports/{id}/cancel", { params: { path: { id } } }),
      ),
    onSettled: () => queryClient.invalidateQueries({ queryKey: [IMPORTS_KEY] }),
  });
}

// One run's report: the test run's prediction, then what the run did. Keyed by
// the run's status and version, so a retest never shows the report it replaced.
// A draft and a test still being read have no report to ask for.
export function useImportReport(
  id: string | null,
  status: ImportRun["status"] | undefined,
  version: number | undefined,
) {
  return useQuery({
    queryKey: [IMPORTS_KEY, "report", id, status, version],
    queryFn:
      id === null ||
      status === undefined ||
      status === "validating" ||
      status === "pending"
        ? skipToken
        : async () =>
            unwrap(
              await api.GET("/imports/{id}/report", {
                params: { path: { id } },
              }),
            ),
  });
}

// A run that lands or reverses wrote records any cached list may hold. Every
// read is asked again once the runs watched have all settled.
export function useLandedRecords(
  statuses: readonly (ImportRun["status"] | undefined)[],
): void {
  const queryClient = useQueryClient();
  const now = statuses.some(writing);
  const seen = useRef(now);
  useEffect(() => {
    const before = seen.current;
    seen.current = now;
    if (before && !now) {
      queryClient.invalidateQueries();
    }
  }, [now, queryClient]);
}
