import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

export type Worklist = components["schemas"]["Worklist"];
export type WorklistItem = components["schemas"]["WorklistItem"];
export type WorklistReason = components["schemas"]["WorklistReason"];
export type WorklistComparison = components["schemas"]["WorklistComparison"];
export type WorklistValue = components["schemas"]["WorklistValue"];
// Derived from the contract's own enums rather than spelled again here: a
// hand-written union goes stale the moment the server adds a value, and it goes
// stale silently — nothing compares the two.
export type WorklistScope = Worklist["scope"];
export type WorklistFilter = NonNullable<Worklist["filter"]>;
export type WorklistCategory = WorklistItem["category"];

export const worklistKey = ["worklist"] as const;

// The whole day in one read, at one scope and one filter.
//
// The key carries both dials, so switching either is a different query rather
// than a refetch of the same one — which is what lets the reader move back to a
// view they have already loaded without watching it reassemble.
export function useWorklist(scope: WorklistScope, filter: WorklistFilter) {
  return useQuery({
    queryKey: [...worklistKey, scope, filter],
    refetchOnWindowFocus: true,
    queryFn: async (): Promise<Worklist> => {
      const { data, error } = await api.GET("/worklist", {
        params: { query: { scope, filter } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}
