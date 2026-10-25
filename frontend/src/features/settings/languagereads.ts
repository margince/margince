import type { QueryClient } from "@tanstack/react-query";
import { reloadReportingFigures } from "../analytics/reporting.queries";

// Reads whose sentences the server writes in the caller's display language.
// They stay cached in the old language until a language change reloads them.
const READER_LANGUAGE_READS = [
  "contact360",
  "company360",
  "reporting-catalog",
  "reporting-card-preview",
  "reporting-comparison",
];

export function reloadReaderLanguageReads(client: QueryClient) {
  return Promise.all([
    client.invalidateQueries({
      predicate: (query) =>
        READER_LANGUAGE_READS.includes(String(query.queryKey[0])),
    }),
    reloadReportingFigures(client),
  ]);
}
