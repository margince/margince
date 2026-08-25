import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
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

// The two verbs the day's decision lane can answer, and the ONE cache key they
// both refresh.
//
// Every mutation here posts to the endpoint that already owns the decision —
// approvals decide at `/approvals`, merges dispose at `/dedupe/candidates`.
// This surface adds no authority of its own, so a verb answered here and the
// same verb answered from the record page take the identical path through the
// server.

// decide answers one staged proposal.
export function useDecideAttention() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      id: string;
      verdict: "approve" | "reject";
    }) => {
      const path =
        input.verdict === "approve"
          ? "/approvals/{id}/approve"
          : "/approvals/{id}/reject";
      const { data, error } = await api.POST(path, {
        params: { path: { id: input.id } },
        ...(input.verdict === "reject" ? { body: { reason: "" } } : {}),
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: attentionKey }),
  });
}

// dispose answers one duplicate pair: merge into the chosen winner, or record
// that the two records are different people after all.
export function useDisposeDuplicate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      id: string;
      disposition: "merge" | "not_a_duplicate";
      winner_id?: string;
    }) => {
      const { data, error } = await api.POST(
        "/dedupe/candidates/{id}/disposition",
        {
          params: { path: { id: input.id } },
          body: { disposition: input.disposition, winner_id: input.winner_id },
        },
      );
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: attentionKey }),
  });
}

// One staged proposal, whole.
//
// The feed sends a lane's worth of each item — a sentence, a kind, a deadline.
// Deciding one needs the rest: the payload a reader may edit, who staged it,
// the evidence behind it. The focus lane shows one card at a time, so this is
// one read per decision rather than a page of them.
export function useApproval(id: string) {
  return useQuery({
    queryKey: ["approvals", "one", id],
    queryFn: async () => {
      const { data, error } = await api.GET("/approvals/{id}", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}
