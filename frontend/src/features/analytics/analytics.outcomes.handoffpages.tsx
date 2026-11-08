// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useInfiniteQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import { Button } from "../../design-system/atoms";
import { PanelRow } from "../../design-system/panel";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";

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
  return useInfiniteQuery({
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
