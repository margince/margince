import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { throwProblem } from "./common";

export const dedupeQueueKey = ["dedupe-candidates"];

/**
 * The open duplicate queue, in one spelling.
 *
 * Exported because the screen is no longer the only reader: chrome that reports
 * what is waiting on a person reads the same queue, and two queries against one
 * path are two answers that can disagree on screen.
 */
export function useDedupeQueue() {
  return useQuery({
    queryKey: dedupeQueueKey,
    queryFn: async () => {
      const { data, error } = await api.GET("/dedupe/candidates", {
        params: { query: { status: "open", limit: 50 } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}
