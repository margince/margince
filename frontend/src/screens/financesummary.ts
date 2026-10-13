import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { unwrap } from "../lib/problem";

// The account's finance summary. It lives here rather than beside the finance
// card because the KPI row reads the SAME figure: one query key, so the two
// readings on a page agree and the second costs no request.
export function useFinanceSummary(companyId: string) {
  return useQuery<components["schemas"]["CompanyFinanceSummary"]>({
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
