import { useMutation, useQuery } from "@tanstack/react-query";
import { useEffect } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { unwrap } from "../../lib/problem";
import "./company360.css";

// The company view's data layer and its right-rail cards.
//
// One read (GET /companies/{id}/360) serves the whole page, and its
// `sections_omitted` is the thing that makes the page honest: a section the
// caller's role cannot read is ABSENT from the payload and named there, so
// every card below can say "hidden from you" instead of drawing an empty
// list that reads as "there is none".

export { AskSection } from "./company360.ask";
export { DealsCard } from "./company360.deals";
export { medianDaysLabel } from "./company360.strip";
export {
  nextCommitmentLine,
  ProposedNextSteps,
  proposesAStep,
  recordNamesIn,
  type SuggestionAction,
  useSuggestionsBody,
} from "./company360.suggestions";

export type Company360 = components["schemas"]["Company360"];

/**
 * useCompany360 reads the whole company page in one round trip.
 *
 * `enabled` lets a caller hold the hook while it asks for nothing. The page
 * does so once the row says archived, because the server refuses that 360.
 * Chrome with no record under it does too: an empty id is a 422.
 */
export function useCompany360(id: string, enabled = true) {
  return useQuery<Company360>({
    queryKey: ["company360", id],
    enabled: enabled && id !== "",
    queryFn: async () => {
      return unwrap(
        await api.GET("/companies/{id}/360", {
          params: { path: { id } },
        }),
      );
    },
  });
}

// VIEW_ACK_DWELL_MS is how long the account must stay open before the visit
// counts. Opening a record and bouncing straight back out is not reading it,
// and an ack from that would mark unread activity as seen.
const VIEW_ACK_DWELL_MS = 5_000;

/**
 * useAcknowledgeCompanyView advances THIS reader's "last seen" baseline
 * for the account — the thing that makes "N new since your last visit" mean
 * anything on the next visit. Without it the server keeps answering with no
 * baseline at all, so every visit reads as the first one.
 *
 * The 360 deliberately does not advance the baseline itself (a prefetch must
 * not be indistinguishable from a visit), so this is the only caller. Leaving
 * before the dwell elapses cancels the timer: the baseline moves only for a
 * visit that actually happened, and when in doubt it stays where it is —
 * showing an item twice is a smaller wrong than hiding one.
 *
 * Success does NOT invalidate the 360. The "new since your last visit" line
 * describes the visit in progress; refetching it out from under the reader
 * would erase the very thing they opened the page to see.
 */
export function useAcknowledgeCompanyView(id: string, visited: boolean) {
  const ack = useMutation({
    mutationFn: async (companyId: string) => {
      unwrap(
        await api.POST("/companies/{id}/view-ack", {
          params: { path: { id: companyId } },
        }),
      );
    },
  });
  // The mutation's own error state holds a failure; nothing renders it. A
  // baseline that did not move costs the reader one repeated line next time,
  // which is not worth an error banner over the account they came to read.
  const fire = ack.mutate;
  useEffect(() => {
    if (!visited) {
      return;
    }
    const timer = window.setTimeout(() => fire(id), VIEW_ACK_DWELL_MS);
    return () => window.clearTimeout(timer);
  }, [id, visited, fire]);
}
