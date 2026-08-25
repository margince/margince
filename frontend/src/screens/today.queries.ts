import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

export type Attention = components["schemas"]["Attention"];
export type AttentionItem = components["schemas"]["AttentionItem"];
export type AttentionLane = "needs_you" | "planned" | "done_for_you";

// Exported because completing a task from this surface invalidates it, and the
// shared task mutation takes the keys to invalidate rather than knowing them.
export const attentionKey = ["attention"] as const;

// The whole day in one read. Every lane arrives together on purpose: a surface
// that fetched three ways would settle three times, and a reader watching lanes
// appear one after another cannot tell "nothing here" from "not here yet".
export function useAttention() {
  return useQuery({
    queryKey: attentionKey,
    refetchOnWindowFocus: true,
    queryFn: async (): Promise<Attention> => {
      const { data, error } = await api.GET("/attention");
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}
