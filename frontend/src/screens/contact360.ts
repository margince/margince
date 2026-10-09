import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { unwrap } from "./common";
import "./contact360.css";

export type Contact360 = components["schemas"]["Contact360"];

/**
 * useContact360 is the contact page's ONE read, so every section describes
 * the same moment rather than a stack of independently-timed round trips.
 *
 * `enabled` is for the callers that are not the page: a surface that only
 * sometimes knows a contact asks under the SAME key, so it reads the page's
 * cache where there is one and opens no request at all where there is not.
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
