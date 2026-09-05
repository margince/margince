// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The account scan as the company page holds it: asked for once on open,
// polled while it runs, and then left alone until the account moves.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

export type AccountScan = components["schemas"]["OrganizationScan"];

/** The query the page and the rows read the scan under. */
export function accountScanKey(orgId: string) {
  return ["account-scan", orgId] as const;
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

/**
 * useAccountScan asks for the reader's scan of one account and keeps it
 * current while a read runs.
 *
 * The ensure fires once per mount: the server compares the account as this
 * reader sees it against what it stored and answers with the stored findings,
 * the same findings marked stale, or a queued read — so an open costs a
 * model call only when the account moved and the hourly floor has passed.
 * `enabled` is false where the page cannot ask at all — an overlay
 * workspace, a 360 that has not answered — so nothing is asked for a page
 * that has nothing to show it on.
 */
export function useAccountScan(
  orgId: string,
  enabled: boolean,
): AccountScan | undefined {
  const client = useQueryClient();
  const scan = useQuery({
    queryKey: accountScanKey(orgId),
    enabled,
    queryFn: async () => {
      const { data, error } = await api.GET("/organizations/{id}/scan", {
        params: { path: { id: orgId } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    refetchInterval: (query) =>
      scanIsLive(query.state.data) ? POLL_LIVE_MS : false,
  });
  const ensure = useMutation({
    mutationKey: accountScanKey(orgId),
    mutationFn: async (id: string) => {
      const { data, error } = await api.POST("/organizations/{id}/scan", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    // The ensure's answer IS the scan as it now stands, so the query takes it
    // rather than re-reading what the server just said. The open's own read
    // is cancelled first: it left before the ensure queued anything, and
    // landing after it would put "never" over "queued" and stop the poll.
    onSuccess: async (data) => {
      await client.cancelQueries({ queryKey: accountScanKey(orgId) });
      client.setQueryData(accountScanKey(orgId), data);
    },
  });
  const fire = ensure.mutate;
  useEffect(() => {
    if (enabled) {
      fire(orgId);
    }
  }, [enabled, orgId, fire]);
  return scan.data;
}
