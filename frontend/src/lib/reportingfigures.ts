import type { QueryClient } from "@tanstack/react-query";

const EVALUATED_FIGURES = [
  "reporting-evaluation",
  "reporting-live",
  "reporting-forecast",
];

// A stale evaluation reloads every figure the page draws, so the next read
// asks against one reading rather than a mix of old and new.
export function reloadReportingFigures(client: QueryClient) {
  return client.invalidateQueries({
    predicate: (query) => EVALUATED_FIGURES.includes(String(query.queryKey[0])),
  });
}
