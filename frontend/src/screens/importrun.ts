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
import { unwrap } from "./common";
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

// A run that is still moving under the server's hand. The commit and the undo
// both run detached, so only a read tells the screen when they finish.
function moving(status: ImportRun["status"] | undefined): boolean {
  return status === "running" || status === "undoing";
}

const POLL_MS = 2000;

// The newest runs first, as the server lists them. Polled while one of them is
// still moving, so the list shows the end of a run the reader walked away from.
export function useRecentImports() {
  return useQuery({
    queryKey: [IMPORTS_KEY, "recent"],
    queryFn: async () =>
      unwrap(await api.GET("/imports", { params: { query: { limit: 20 } } }))
        .data,
    refetchInterval: (query) =>
      query.state.data?.some((one) => moving(one.status)) ? POLL_MS : false,
  });
}

// One run, read again every two seconds while it is moving.
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
      moving(query.state.data?.status) ? POLL_MS : false,
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
    onSuccess: () => queryClient.invalidateQueries({ queryKey: [IMPORTS_KEY] }),
  });
}

// One run's report: the test run's prediction, then what the run did. Keyed by
// the run's status, so each move of the run reads it again.
export function useImportReport(
  id: string | null,
  status: ImportRun["status"] | undefined,
) {
  return useQuery({
    queryKey: [IMPORTS_KEY, "report", id, status],
    queryFn:
      id === null || status === undefined
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
  const now = statuses.some(moving);
  const seen = useRef(now);
  useEffect(() => {
    const before = seen.current;
    seen.current = now;
    if (before && !now) {
      queryClient.invalidateQueries();
    }
  }, [now, queryClient]);
}
