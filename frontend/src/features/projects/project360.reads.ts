// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import { projectRowKey } from "../../lib/activitykeys";
import { unwrap } from "../../lib/problem";
import { project360Key } from "./project360.key";

// `enabled` holds the read once the row says archived, because the server
// refuses that 360.
export function useProject360(id: string, enabled = true) {
  return useQuery({
    queryKey: project360Key(id),
    enabled,
    queryFn: async ({ signal }) => {
      return unwrap(
        await api.GET("/projects/{id}/360", {
          signal,
          params: { path: { id } },
        }),
      );
    },
  });
}

// A restore from the archived page re-reads the row through its key's prefix,
// and a restore elsewhere reaches it because the row is a live read.
function useProjectRow(id: string, enabled: boolean) {
  return useQuery({
    queryKey: projectRowKey(id),
    enabled,
    queryFn: async ({ signal }) => {
      return unwrap(
        await api.GET("/projects/{id}", {
          signal,
          params: { path: { id } },
        }),
      );
    },
  });
}

/**
 * The project page's reads. A live project answers its 360, so it costs that
 * one request. The row is read only once the 360 is refused, to tell an
 * archived project from a missing one. An archived row then holds the 360.
 */
export function useProjectScreenReads(id: string) {
  // The row's cache entry, observed without reading it: the 360 is declared
  // first because its refusal is what turns the row read on.
  const known = useProjectRow(id, false);
  const view = useProject360(id, !known.data?.archived_at);
  const row = useProjectRow(id, view.isError);
  // A refused 360 may be an archive the row will report, so the page waits
  // for the row rather than showing the refusal.
  const waiting = view.isError && row.isPending;
  return {
    view: waiting ? { ...view, isPending: true, isError: false } : view,
    row,
  };
}
