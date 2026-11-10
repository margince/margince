// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useInfiniteQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { api } from "../../api/client";
import { Button } from "../../design-system/atoms";
import { PanelRow } from "../../design-system/panel";
import { useT } from "../../i18n";
import { problemFieldErrorsOf, unwrap } from "../../lib/problem";
import { REPORTING_EVALUATION_KEY } from "../../lib/reportingfigures";

/**
 * Re-pins the SDR reads at a fresh instant once the server refuses their
 * `as_of` as too old. The evaluation they pair with is read again too, so the
 * summary, its rows and the lead readings still describe one moment.
 */
export function useRepinWhenAsOfExpires(error: unknown) {
  const client = useQueryClient();
  const expired = problemFieldErrorsOf(error).some(
    (refusal) => refusal.field === "as_of" && refusal.code === "too_old",
  );
  useEffect(() => {
    if (!expired) return;
    void client.invalidateQueries({
      queryKey: [...REPORTING_EVALUATION_KEY, "my-leads"],
    });
    void client.invalidateQueries({ queryKey: ["sdr-summary"] });
  }, [expired, client]);
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
  useRepinWhenAsOfExpires(pages.error);
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
