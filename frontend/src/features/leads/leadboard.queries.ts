// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { ListQuery } from "../../components/listquery";
import type { MessageKey } from "../../i18n/en";
import { LEAD_STATUS_COUNTS_KEY, leadTerminalKey } from "../../lib/leadkeys";
import { throwProblem, unwrap } from "../../lib/problem";
import type { Lead } from "./leadpresentation";
import { leadsQueryParams } from "./leads.list.queries";

// The dials the leads-by-status report filters by, under the names /leads
// gives them.
const COUNTED_DIALS = ["status", "owner_id", "source", "origin"] as const;

// The list's own drawing dials, which choose an order or widen to archived rows.
// The report counts archived leads whatever the list shows, and order is no count.
const UNCOUNTED_DRAWING = new Set(["sort", "include_archived"]);

// The reason each uncountable dial gives, first match told. A dial missing here
// still withholds, under the catch-all reason.
const WITHHELD_DIALS: readonly (readonly [string, MessageKey])[] = [
  ["q", "lead.boardCountsNoSearch"],
  ["tag_id", "lead.boardCountsNoTagFilter"],
  ["owner_team_id", "lead.boardCountsNoTeamFilter"],
  ["unassigned", "lead.boardCountsNoTeamFilter"],
  ["min_score", "lead.boardCountsNoScoreFilter"],
  ["sla_state", "lead.boardCountsNoResponseFilter"],
  ["industry", "lead.boardCountsNoCompanyFilter"],
  ["country", "lead.boardCountsNoCompanyFilter"],
  ["size_band", "lead.boardCountsNoCompanyFilter"],
];

function narrowingDials(query: ListQuery): Map<string, unknown> {
  return new Map(
    Object.entries(leadsQueryParams(query)).filter(
      ([dial, value]) =>
        !UNCOUNTED_DRAWING.has(dial) &&
        value !== undefined &&
        value !== "" &&
        !(Array.isArray(value) && value.length === 0),
    ),
  );
}

function isCounted(dial: string): boolean {
  return COUNTED_DIALS.some((counted) => counted === dial);
}

// The counts query's `enabled` and the column's words both read this, so the
// two cannot disagree.
export function terminalCountsWithheldBecause(
  query: ListQuery,
): MessageKey | undefined {
  const uncounted = [...narrowingDials(query).keys()].filter(
    (dial) => !isCounted(dial),
  );
  if (uncounted.length === 0) return undefined;
  const named = WITHHELD_DIALS.find(([dial]) => uncounted.includes(dial));
  return named?.[1] ?? "lead.boardCountsNoOtherFilter";
}

// The one reason the server decides: an owner dial naming somebody whose
// counts this reader may not measure answers 403.
export function terminalCountsRefusedBecause(
  counts: Map<string, number> | null | undefined,
): MessageKey | undefined {
  return counts === null ? "lead.boardCountsOwnerNotMeasurable" : undefined;
}

export function leadStatusReportFilters(
  query: ListQuery,
): Record<string, unknown> {
  const dials = narrowingDials(query);
  return Object.fromEntries(
    COUNTED_DIALS.flatMap((dial) =>
      dials.has(dial) ? [[dial, dials.get(dial)]] : [],
    ),
  );
}

// The one lead read that counts archived leads. Under ["leads"], so every lead
// mutation refreshes the terminal figures with the cards.
export function useLeadStatusCounts(query: ListQuery) {
  const filters = leadStatusReportFilters(query);
  return useQuery({
    queryKey: [...LEAD_STATUS_COUNTS_KEY, filters],
    enabled: terminalCountsWithheldBecause(query) === undefined,
    queryFn: async (): Promise<Map<string, number> | null> => {
      const { data, error, response } = await api.POST("/reports/{report}", {
        params: { path: { report: "leads-by-status" } },
        body: {
          group_by: ["status"],
          aggregates: [{ fn: "count", as: "leads" }],
          filters,
        },
      });
      if (error) {
        if (response.status === 403) return null;
        throwProblem(error);
      }
      const counts = new Map<string, number>();
      for (const row of data.rows) {
        const status = row.status;
        const leads = row.leads;
        if (typeof status === "string" && typeof leads === "number") {
          counts.set(status, leads);
        }
      }
      return counts;
    },
  });
}

// The /leads query for one terminal column: the list's dials, held to that
// status and widened to the archived rows every terminal lead is.
export function terminalLeadsQueryParams(
  query: ListQuery,
  status: Lead["status"],
) {
  return { ...leadsQueryParams(query), status, include_archived: true };
}

// The leads in one terminal column, read only while it is open. A status dial
// naming another status leaves the column empty, as the report's count says.
export function useTerminalLeads(
  query: ListQuery,
  status: Lead["status"],
  enabled: boolean,
) {
  const params = terminalLeadsQueryParams(query, status);
  const dialStatus = query.filters.status;
  return useInfiniteQuery({
    queryKey: [...leadTerminalKey(status), params],
    enabled: enabled && (!dialStatus || dialStatus === status),
    initialPageParam: null as string | null,
    queryFn: async ({ pageParam }) => {
      return unwrap(
        await api.GET("/leads", {
          params: {
            query: { ...params, limit: 50, cursor: pageParam ?? undefined },
          },
        }),
      );
    },
    // A column whose count runs to thousands stays reachable past its first
    // fifty, or its own head would contradict the list under it.
    getNextPageParam: (last) =>
      last.page.has_more ? (last.page.next_cursor ?? null) : null,
  });
}
