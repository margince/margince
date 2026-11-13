// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The account scan as the company page holds it: asked for once on open,
// polled while it runs, and then left alone until the account moves.

import {
  type QueryClient,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { unwrap } from "../../lib/problem";

export type AccountScan = components["schemas"]["CompanyScan"];

/** The query the page and the rows read the scan under. */
export function accountScanKey(companyId: string) {
  return ["account-scan", companyId] as const;
}

/** Whether a read is in flight: the states the page keeps polling under. */
export function scanIsLive(scan: AccountScan | undefined): boolean {
  return scan?.state === "queued" || scan?.state === "running";
}

/** Whether the read has settled and its findings are the ones to draw. */
export function scanHasSettled(scan: AccountScan | undefined): boolean {
  return (
    scan?.state === "done" ||
    scan?.state === "degraded" ||
    scan?.state === "failed"
  );
}

// The poll while a read is in flight. Three seconds, like the site read's:
// slow enough to cost nothing, fast enough that the pending row gives way
// the moment the findings land.
const POLL_LIVE_MS = 3000;

// ENSURE_SHARED_MS is how long one ensure answers for every consumer of its
// account. It covers panels mounting on one page load. Past it, an open asks
// again, and the server's own floor keeps that from costing a model call.
export const ENSURE_SHARED_MS = 10_000;

// The open itself, held in the query cache so sign-out drops it with every
// other read. Its own key: a refresh of the scan must not mark it as unasked.
function accountOpenKey(companyId: string) {
  return ["account-scan-open", companyId] as const;
}

// openAccount sends the ensure for this account, or hands back the one already
// sent within the window. The answer is the scan as it now stands. The query
// takes it before anyone waiting on the open hears back.
function openAccount(
  client: QueryClient,
  companyId: string,
): Promise<AccountScan> {
  const key = accountScanKey(companyId);
  return client.fetchQuery({
    queryKey: accountOpenKey(companyId),
    staleTime: ENSURE_SHARED_MS,
    retry: false,
    queryFn: async ({ signal }) => {
      const markedBefore = client.getQueryState(key)?.isInvalidated ?? false;
      const scan = unwrap(
        await api.POST("/companies/{id}/scan", {
          params: { path: { id: companyId } },
          signal,
        }),
      );
      // Aborted when sign-out removed the open: the answer was the last
      // reader's and must not land in the next one's cache.
      signal.throwIfAborted();
      // A change on the record while the open was out marked the scan for a
      // new read. That read is newer than this answer, so it is left to run.
      if (!markedBefore && client.getQueryState(key)?.isInvalidated) {
        return scan;
      }
      // A poll still out left before the ensure queued anything. Landing after
      // it would put "never" over "queued" and stop the poll.
      await client.cancelQueries({ queryKey: key });
      signal.throwIfAborted();
      client.setQueryData(key, scan);
      return scan;
    },
  });
}

/**
 * useAccountScan asks for the reader's scan of one account and keeps it
 * current while a read runs.
 *
 * Each mount opens the account once. Consumers mounting together share one
 * open, and the read waits for it, then serves the polls and refreshes.
 * `enabled` is false where the page has nothing to show the scan on.
 */
export function useAccountScan(
  companyId: string,
  enabled: boolean,
): AccountScan | undefined {
  const client = useQueryClient();
  const open = useMutation({
    mutationFn: (id: string) => openAccount(client, id),
  });
  // This mount's open has its answer, or its refusal, for this account. Until
  // then a read would assemble the account a second time to say the same.
  const opened =
    open.variables === companyId && (open.isSuccess || open.isError);
  const scan = useQuery({
    queryKey: accountScanKey(companyId),
    enabled: enabled && opened,
    // The open's answer is current: read again only to poll, or on a change.
    staleTime: ENSURE_SHARED_MS,
    queryFn: async () => {
      return unwrap(
        await api.GET("/companies/{id}/scan", {
          params: { path: { id: companyId } },
        }),
      );
    },
    refetchInterval: (query) =>
      scanIsLive(query.state.data) ? POLL_LIVE_MS : false,
  });
  const fire = open.mutate;
  useEffect(() => {
    if (enabled) {
      fire(companyId);
    }
  }, [enabled, companyId, fire]);
  return scan.data;
}
