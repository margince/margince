import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { unwrap } from "../lib/problem";

// The account's finance summary. It lives here rather than beside the finance
// card because the KPI row reads the same figure. One query key keeps the two
// readings on a page in agreement, and the second costs no request.
export function useFinanceSummary(companyId: string, enabled = true) {
  return useQuery<components["schemas"]["CompanyFinanceSummary"]>({
    enabled,
    queryKey: ["finance-summary", companyId],
    queryFn: async () => {
      return unwrap(
        await api.GET("/companies/{id}/finance-summary", {
          params: { path: { id: companyId } },
        }),
      );
    },
  });
}

type FinanceSummary = components["schemas"]["CompanyFinanceSummary"];

/**
 * The account's overdue money as every surface reads it: the company banner,
 * the payment health line and the Finance panel. One rule for when it shows.
 */
export type OverdueReading = Readonly<{
  amountMinor: number;
  currency: string;
  // Absent from an older server, which sends the amount alone.
  count?: number;
  oldestDays?: number;
  dunningLevel?: number;
  // Set when the figures are the last good sync's. The source is stale or its
  // last refresh failed, so an invoice may be paid since.
  asOf?: string;
}>;

// Null when nothing is overdue or the figure has no sync to vouch for it. A
// stale or failed source still shows, dated, because the last good answer is
// what we know.
export function overdueReadingOf(
  summary: FinanceSummary | undefined,
): OverdueReading | null {
  const overdue = summary?.overdue;
  if (
    !summary?.last_synced_at ||
    overdue?.amount_minor == null ||
    overdue.amount_minor <= 0 ||
    !overdue.currency
  ) {
    return null;
  }
  if (
    summary.state !== "connected" &&
    summary.state !== "stale" &&
    summary.state !== "error"
  ) {
    return null;
  }
  return {
    amountMinor: overdue.amount_minor,
    currency: overdue.currency,
    count: summary.overdue_count ?? undefined,
    oldestDays: summary.oldest_overdue_days ?? undefined,
    dunningLevel: summary.dunning_level ?? undefined,
    asOf: summary.state === "connected" ? undefined : summary.last_synced_at,
  };
}
