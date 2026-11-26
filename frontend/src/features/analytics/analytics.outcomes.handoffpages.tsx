// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryClient,
  useInfiniteQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect } from "react";
import { api } from "../../api/client";
import { Button } from "../../design-system/atoms";
import { PanelRow } from "../../design-system/panel";
import { useT } from "../../i18n";
import { problemFieldErrorsOf, unwrap } from "../../lib/problem";
import { REPORTING_EVALUATION_KEY } from "../../lib/reportingfigures";

// The refusals a fresh instant cures. The instant fell out of the server's
// window, or the server's clock runs behind the one that stamped it.
const REPINNED_CODES = new Set(["too_old", "future"]);

// The instants already re-pinned, per client, so one refusal re-pins once
// however often the refused read answers it again.
const repinned = new WeakMap<QueryClient, Set<string>>();

/**
 * Re-pins the SDR reads once the server refuses their `as_of`. The evaluation
 * they pair with is read again, and its new instant names a fresh summary. A
 * summary read at now is read again too. A read under the refused instant is
 * never asked again.
 */
export function useRepinWhenAsOfExpires(error: unknown, asOf?: string) {
  const client = useQueryClient();
  const refused =
    asOf !== undefined &&
    problemFieldErrorsOf(error).some(
      (refusal) =>
        refusal.field === "as_of" && REPINNED_CODES.has(refusal.code),
    );
  const instant = refused ? asOf : undefined;
  useEffect(() => {
    if (instant === undefined) return;
    const done = repinned.get(client) ?? new Set<string>();
    repinned.set(client, done);
    if (done.has(instant)) return;
    done.add(instant);
    void client.invalidateQueries({
      queryKey: [...REPORTING_EVALUATION_KEY, "my-leads"],
    });
    void client.invalidateQueries({
      predicate: (query) =>
        query.queryKey[0] === "sdr-summary" &&
        query.queryKey.length === 3 &&
        query.queryKey[2] === null,
    });
  }, [instant, client]);
}

/**
 * Whose open handoffs to list: one seat, a team, or the whole company. No
 * kind is the seat's own default population, which My teams is.
 */
export type HandoffScope = Readonly<{
  kind?: "owner" | "team" | "workspace";
  id?: string;
}>;

/** The open handoffs of a scope in one state, one page at a time. */
export function useHandoffPages(
  scope: HandoffScope,
  state: "waiting" | "returned",
  enabled: boolean,
  // The summary's instant, so the rows describe the totals beside them.
  asOf?: string,
) {
  const pages = useInfiniteQuery({
    queryKey: [
      "sdr-summary",
      scope.id ?? scope.kind ?? "default",
      "handoffs",
      state,
      asOf ?? null,
    ],
    enabled,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) =>
      unwrap(
        await api.GET("/analytics/sdr-summary/handoffs", {
          params: {
            query: {
              ...(scope.kind ? { scope_kind: scope.kind } : {}),
              ...(scope.id ? { scope_id: scope.id } : {}),
              state,
              ...(pageParam ? { cursor: pageParam } : {}),
              ...(asOf ? { as_of: asOf } : {}),
            },
          },
          signal,
        }),
      ),
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
  useRepinWhenAsOfExpires(pages.error, asOf);
  return pages;
}

export function MoreRows({
  rows,
}: Readonly<{ rows: ReturnType<typeof useHandoffPages> }>) {
  const t = useT();
  if (!rows.hasNextPage) return null;
  return (
    <PanelRow>
      <Button
        onClick={() => void rows.fetchNextPage()}
        disabled={rows.isFetchingNextPage}
      >
        {t("analytics.sdrShowMore")}
      </Button>
    </PanelRow>
  );
}
