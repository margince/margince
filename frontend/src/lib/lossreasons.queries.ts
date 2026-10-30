import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useT } from "../i18n";
import { throwProblem } from "./problem";
import { withShippedLabels } from "./shippedlabels";

// The loss-reason list a closer picks from, and the review that maps the notes
// of deals lost before it to a reason. The close dialog, the settings cards and
// the report labels all read the list through here.

export type LossReason = components["schemas"]["LossReason"];
export type LegacyLossReason = components["schemas"]["LegacyLossReason"];

export const LOSS_REASONS_KEY = ["loss-reasons"] as const;
const LEGACY_LOSS_REASONS_KEY = ["loss-reasons", "legacy"] as const;

export function useLossReasons(enabled = true) {
  const t = useT();
  return useQuery({
    queryKey: LOSS_REASONS_KEY,
    enabled,
    queryFn: async () => {
      const { data, error, response } = await api.GET("/loss-reasons");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data.data;
    },
    select: (reasons) => withShippedLabels(reasons, t),
  });
}

/** The reasons a new loss may name: the retired ones stay readable only. */
export function choosableLossReasons(
  reasons: readonly LossReason[] | undefined,
): LossReason[] {
  return (reasons ?? []).filter((reason) => reason.active);
}

export function useCreateLossReason() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (
      body: components["schemas"]["CreateLossReasonRequest"],
    ) => {
      const { data, error, response } = await api.POST("/loss-reasons", {
        body,
      });
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: LOSS_REASONS_KEY }),
  });
}

export function useUpdateLossReason() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (args: {
      id: string;
      body: components["schemas"]["UpdateLossReasonRequest"];
    }) => {
      const { data, error, response } = await api.PATCH("/loss-reasons/{id}", {
        params: { path: { id: args.id } },
        body: args.body,
      });
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: LOSS_REASONS_KEY }),
  });
}

export function useLegacyLossReasons(enabled: boolean) {
  return useQuery({
    queryKey: LEGACY_LOSS_REASONS_KEY,
    enabled,
    queryFn: async ({ signal }) => {
      const { data, error, response } = await api.GET("/loss-reasons/legacy", {
        params: { query: { limit: 50 } },
        signal,
      });
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

export function useDecideLegacyLossReason() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (args: { textHash: string; reasonKey: string }) => {
      const { data, error, response } = await api.PUT(
        "/loss-reasons/legacy/{text_hash}",
        {
          params: { path: { text_hash: args.textHash } },
          body: { reason_key: args.reasonKey },
        },
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: LEGACY_LOSS_REASONS_KEY }),
  });
}

export function useApplyLegacyLossReasons() {
  const qc = useQueryClient();
  return useMutation({
    // The decisions the reader saw, at the versions shown: the server applies
    // those and refuses one that has moved since.
    mutationFn: async (shown: readonly LegacyLossReason[]) => {
      const mappings = shown.flatMap((row) =>
        row.mapping
          ? [{ id: row.mapping.id, version: row.mapping.version }]
          : [],
      );
      const { data, error, response } = await api.POST(
        "/loss-reasons/legacy/apply",
        { body: { mappings } },
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: LOSS_REASONS_KEY }),
  });
}

/** The rows an apply would change: decided, with deals still to change. */
export function pendingDecisions(
  rows: readonly LegacyLossReason[],
): LegacyLossReason[] {
  return rows.filter((row) => row.mapping && row.pending_count > 0);
}

/**
 * How a lost deal says why: the label of its reason, retired ones included,
 * and its note when it has one. A code the list no longer holds reads as the
 * code. Undefined when the deal says nothing.
 */
export function lostReasonWords(
  deal: Readonly<{
    lost_reason?: string | null;
    lost_reason_code?: string | null;
  }>,
  reasons: readonly LossReason[] | undefined,
): string | undefined {
  const code = deal.lost_reason_code ?? undefined;
  const label = code
    ? (reasons?.find((reason) => reason.key === code)?.label ?? code)
    : undefined;
  const words = [label, deal.lost_reason?.trim() || undefined].filter(Boolean);
  return words.length > 0 ? words.join(" — ") : undefined;
}
