import type { QueryClient } from "@tanstack/react-query";

/**
 * The prefix every reporting evaluation is cached under. A write to a figure a
 * report reads, such as a deal's product allocations, invalidates it.
 */
export const REPORTING_EVALUATION_KEY = ["reporting-evaluation"] as const;

const EVALUATED_FIGURES: readonly string[] = [
  ...REPORTING_EVALUATION_KEY,
  "reporting-live",
  "reporting-forecast",
  "sdr-summary",
];

// A stale evaluation reloads every figure the page draws, so the next read
// asks against one reading rather than a mix of old and new.
export async function reloadReportingFigures(client: QueryClient) {
  await Promise.all([
    client.invalidateQueries({
      queryKey: ["sales-assessment"],
      refetchType: "none",
    }),
    client.invalidateQueries({
      predicate: (query) =>
        EVALUATED_FIGURES.includes(String(query.queryKey[0])),
    }),
  ]);
}
