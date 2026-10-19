import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { leadScoreKey } from "../../lib/leadkeys";
import { unwrap } from "../../lib/problem";

type Lead = components["schemas"]["Lead"];

/**
 * The lead's score breakdown. Every reader of it shares this read, which does
 * not run for an archived lead: the server refuses the breakdown there, and a
 * reader of the hook would only draw a section that failed to load.
 */
export function useLeadScore(lead: Pick<Lead, "id" | "archived_at">) {
  return useQuery({
    queryKey: leadScoreKey(lead.id),
    enabled: !lead.archived_at,
    queryFn: async () => {
      return unwrap(
        await api.GET("/leads/{id}/score", {
          params: { path: { id: lead.id } },
        }),
      );
    },
  });
}
