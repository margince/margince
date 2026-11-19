import {
  type InfiniteData,
  type UseInfiniteQueryResult,
  useInfiniteQuery,
} from "@tanstack/react-query";
import { api, FIRST_PAGE } from "../api/client";
import type { components } from "../api/schema";
import type { EntityKind } from "./entity";
import { unwrap } from "./problem";

type AuditHistoryListResponse =
  components["schemas"]["AuditHistoryListResponse"];

/** A record's audit trail, a page at a time, newest first. */
export function useRecordHistory(
  kind: EntityKind,
  id: string,
  // Off for a caller that reads one event on a record where it never happened.
  enabled = true,
  // One verb, so a caller after one event does not page the whole trail.
  action?: string,
): UseInfiniteQueryResult<InfiniteData<AuditHistoryListResponse>> {
  return useInfiniteQuery({
    // The verb is in the key: a filtered read and the whole trail are
    // different answers.
    queryKey: ["record-history", kind, id, action ?? null],
    enabled,
    initialPageParam: FIRST_PAGE,
    queryFn: async ({ pageParam }) => {
      return unwrap(
        await api.GET("/records/{entity_type}/{id}/history", {
          params: {
            path: { entity_type: kind, id },
            query: {
              limit: 20,
              ...(pageParam ? { cursor: pageParam } : {}),
              ...(action ? { action } : {}),
            },
          },
        }),
      );
    },
    getNextPageParam: (last) => last?.page?.next_cursor ?? null,
  });
}
