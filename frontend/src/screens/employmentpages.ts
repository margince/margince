import { useInfiniteQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";
import { ENTITY_NAME_KEY, fetchEntityName } from "./entityref";

type View = components["schemas"]["Contact360"];
type Employment = components["schemas"]["Contact360Employment"];

export function useEmploymentPages(view: View) {
  const id = view.contact.id;
  const client = useQueryClient();
  return useInfiniteQuery({
    queryKey: ["contactEmployments", id, view.employments?.page.next_cursor],
    enabled: Boolean(view.employments?.page.has_more),
    initialPageParam: view.employments?.page.next_cursor ?? "",
    queryFn: async ({ pageParam }) => {
      const { data, error } = await api.GET("/relationships", {
        params: {
          query: {
            contact_id: id,
            kind: "employment",
            cursor: pageParam,
            limit: 50,
          },
        },
      });
      if (error) throwProblem(error);
      const companies = new Map(
        await Promise.all(
          [
            ...new Set(
              data.data.flatMap((row) =>
                row.company_id ? [row.company_id] : [],
              ),
            ),
          ].map(
            async (company) =>
              [
                company,
                await client.fetchQuery({
                  queryKey: ["company", ENTITY_NAME_KEY, company],
                  queryFn: () => fetchEntityName("company", company),
                }),
              ] as const,
          ),
        ),
      );
      const roles: Employment[] = [];
      for (const row of data.data) {
        if (!row.company_id) continue;
        roles.push({
          relationship_id: row.id,
          company_id: row.company_id,
          company_name: companies.get(row.company_id),
          role: row.role,
          is_current_primary: row.is_current_primary ?? false,
          employment_status: row.employment_status,
          started_at: row.started_at,
          ended_at: row.ended_at,
          started_precision: row.started_precision,
          ended_precision: row.ended_precision,
          version: row.version,
        });
      }
      return { roles, page: data.page };
    },
    getNextPageParam: (last) =>
      last.page.has_more ? (last.page.next_cursor ?? undefined) : undefined,
  });
}
