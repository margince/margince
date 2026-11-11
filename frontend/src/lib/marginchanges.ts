// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The changes Margince made to deals: the reader's open ones, one deal's list,
// the team's totals, and the Undo of many deals at once.

import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { unwrap } from "./problem";
import { invalidateHandledChange, worklistKey } from "./workkeys";

export type MarginceChangeSummary =
  components["schemas"]["MarginceChangeSummary"];
export type MarginceChangeBundle =
  components["schemas"]["MarginceChangeBundle"];
export type UndoMarginceChangeResult =
  components["schemas"]["UndoMarginceChangeResult"];
export type TeamMarginceChanges = components["schemas"]["TeamMarginceChanges"];
export type TeamMarginceOwnerlessDeal =
  components["schemas"]["TeamMarginceOwnerlessDeal"];
export type DealMarginceCheck = components["schemas"]["DealMarginceCheck"];

/** The reader's open changes, one bundle per deal. */
export function usePendingMarginceChanges() {
  return useQuery(pendingChangesQuery());
}

/** One deal's changes, newest first, a page at a time. */
export function useDealMarginceChanges(dealId: string) {
  return useInfiniteQuery({
    queryKey: ["deals", dealId, "margince-changes"],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api.GET("/deals/{id}/margince-changes", {
          params: {
            path: { id: dealId },
            query: pageParam ? { cursor: pageParam } : {},
          },
        }),
      ),
    getNextPageParam: (last) => last.next_cursor,
  });
}

/** What Margince changed on the reader's team. Only a lead may ask. */
export function useTeamMarginceChanges(enabled: boolean) {
  return useQuery({
    queryKey: [...worklistKey, "team", "margince-changes"],
    enabled,
    queryFn: async (): Promise<TeamMarginceChanges> =>
      unwrap(await api.GET("/worklist/team/changes")),
  });
}

/** The Undo of whole deals; each bundle is one deal. */
export function bundleItem(
  bundle: MarginceChangeBundle,
  archiveConfirmedFor?: string,
) {
  return {
    deal_id: bundle.deal_id,
    version: bundle.version,
    audit_log_ids: bundle.audit_log_ids,
    ...(archiveConfirmedFor
      ? { archive_confirmed_for: archiveConfirmedFor }
      : {}),
  };
}

/** The team's work an Undo of an opened or reopened deal asks about first. */
export function archiveQuestion(
  result: UndoMarginceChangeResult,
): { notes: number; tasks: number; work: string; kind: string } | null {
  const asked = result.kept.find(
    (kept) => kept.reason === "needs_confirmation",
  );
  return asked
    ? {
        notes: asked.team_notes ?? 0,
        tasks: asked.team_tasks ?? 0,
        work: asked.team_work ?? "",
        kind: asked.kind ?? "",
      }
    : null;
}

// The route takes at most this many deals per call.
const UNDO_BATCH = 200;

/**
 * What a run of Undo batches came back with. `results` holds every deal a
 * batch answered for. `unsent` holds the deals of the failed batch and of every
 * batch after it. The server may have committed the failed one, so a retry
 * reads the deals again before it sends them. `settled` holds the unsent deals
 * that the pending read no longer lists, with no answer of their own.
 */
export type BundleUndoRun = {
  results: UndoMarginceChangeResult[];
  unsent: MarginceChangeBundle[];
  settled?: MarginceChangeBundle[];
  error?: unknown;
};

/** Undoes deal bundles and refreshes everything that shows them. */
export function useUndoMarginceChanges() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async ({
      bundles,
      archiveConfirmedFor,
    }: {
      bundles: readonly MarginceChangeBundle[];
      archiveConfirmedFor?: string;
    }): Promise<BundleUndoRun> => {
      const results: UndoMarginceChangeResult[] = [];
      for (let at = 0; at < bundles.length; at += UNDO_BATCH) {
        try {
          const answer = unwrap(
            await api.POST("/deal-corrections/undo", {
              body: {
                items: bundles
                  .slice(at, at + UNDO_BATCH)
                  .map((bundle) => bundleItem(bundle, archiveConfirmedFor)),
              },
            }),
          );
          results.push(...answer.results);
        } catch (error) {
          return { results, unsent: bundles.slice(at), error };
        }
      }
      return { results, unsent: [] };
    },
    onSettled: () => invalidateHandledChange(client),
  });
}

/**
 * The deals of `unsent` to send again, at the version the server reads now.
 * Only the changes the reader first chose go again. A change that arrived
 * since is never taken back unseen. A deal the read left out has settled. If
 * the read was cut short, the deal goes again as it was and the server answers
 * for it.
 */
export function stillPending(
  unsent: readonly MarginceChangeBundle[],
  fresh: MarginceChangeSummary | undefined,
): MarginceChangeBundle[] {
  const fresher = new Map(
    (fresh?.bundles ?? []).map((bundle) => [bundle.deal_id, bundle]),
  );
  return unsent.flatMap((bundle) => {
    const now = fresher.get(bundle.deal_id);
    if (!now) return fresh?.truncated ? [bundle] : [];
    const chosen = new Set(bundle.audit_log_ids);
    const audits = now.audit_log_ids.filter((id) => chosen.has(id));
    return audits.length > 0 ? [{ ...now, audit_log_ids: audits }] : [];
  });
}

/** The pending read, fetched fresh for a retry. */
export function pendingChangesQuery() {
  return {
    queryKey: [...worklistKey, "margince-changes"],
    queryFn: async (): Promise<MarginceChangeSummary> =>
      unwrap(await api.GET("/deal-corrections/pending")),
  };
}

/**
 * The changes an Undo of this bundle may send: those the server still allows,
 * and only those the reader was shown. An older server names no undoable
 * subset, so every listed change that was shown stands in.
 */
export function shownUndoable(
  bundle: MarginceChangeBundle,
  shown: ReadonlySet<string>,
): string[] {
  return (bundle.undoable_audit_log_ids ?? bundle.audit_log_ids).filter((id) =>
    shown.has(id),
  );
}
