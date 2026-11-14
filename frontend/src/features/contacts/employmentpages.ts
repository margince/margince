import { useInfiniteQuery, useQueryClient } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { companyReferenceQuery } from "../../components/entityref.queries";
import {
  nextEmploymentCursor,
  readEmploymentPage,
} from "../../lib/employmentpages";

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
      const data = await readEmploymentPage(id, pageParam);
      const companies = new Map(
        await Promise.all(
          [...new Set(data.roles.map((row) => row.company_id))].map(
            async (company) =>
              [
                company,
                (await client.fetchQuery(companyReferenceQuery(company)))
                  ?.name ?? null,
              ] as const,
          ),
        ),
      );
      const roles: Employment[] = data.roles.map((row) => ({
        ...row,
        company_name: companies.get(row.company_id),
      }));
      return { roles, page: data.page };
    },
    getNextPageParam: (last) => nextEmploymentCursor(last.page),
  });
}
