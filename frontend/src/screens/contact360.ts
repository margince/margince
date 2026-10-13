import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { unwrap } from "../lib/problem";
import "./contact360.css";

export type Contact360 = components["schemas"]["Contact360"];

/**
 * useContact360 is the contact page's ONE read, so every section describes
 * the same moment rather than a stack of independently-timed round trips.
 *
 * `enabled` lets a caller hold the hook while it asks for nothing. The page
 * does so once the row says archived, because the server refuses that 360. A
 * surface that only sometimes knows a contact reads the page's cache, or nothing.
 */
export function useContact360(id: string, enabled = true) {
  return useQuery({
    enabled,
    queryKey: ["contact360", id],
    queryFn: async () => {
      return unwrap(
        await api.GET("/contacts/{id}/360", {
          params: { path: { id } },
        }),
      );
    },
  });
}

/** omitted reports whether a section was withheld for lack of a grant. */
export function omitted(
  view: Contact360 | undefined,
  section: string,
): boolean {
  return Boolean(view?.sections_omitted?.some((s) => s === section));
}
