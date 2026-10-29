import { type QueryClient, useQuery } from "@tanstack/react-query";
import { useEffect } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import type { QueryLike } from "../design-system/querystates";
import { unwrap } from "./problem";
import {
  briefKey,
  digestKey,
  isLiveWorkRead,
  magicKey,
  worklistKey,
} from "./workkeys";

// The approvals query-hook family.
//
// TWO names, and which one belongs where is the whole rule: the surface a reader
// meets is **Decisions** — that is the panel title, and every `decision.*` copy
// key — and the thing on the wire is an **approval**, which is what `crm.yaml`
// calls it and therefore what this file, its hooks and its types are named for.
// Nothing is called an inbox, a drafts queue or a staged list: those were three
// more names for one surface, and a reader who is told four names for one thing
// has been told none. This file was `inbox.queries.ts` and was the last of them
// in the tree.
//
// The Decided view is an Option-1 client-side partition:
// the listApprovals contract only accepts status ∈ {pending, approved,
// rejected} (crm.yaml enum), and the server computes expiry LAZILY at read
// time (approvals/inbox.go effectiveStatus) — a pending row past its expiry
// is STORED status=pending but WIRED back as status="expired". So there is no
// status=expired query; expired items ride in on the status=pending response
// and are re-partitioned here. The merged hooks return a plain QueryLike (the
// narrow surface QueryGate reads) — no `as unknown as UseQueryResult` lie.

export type Approval = components["schemas"]["Approval"];
export type ApprovalStatus = "pending" | "approved" | "rejected";
export type ApprovalPage = { data: Approval[] };

// What one bundle decision did, member by member. The route answers 200 with a
// per-member outcome rather than a single verdict, because the members were
// always independent authorities: one that expired, one somebody else already
// answered and one whose follow-on change failed each report for themselves
// while their siblings decide normally.
export type BundleDecision = components["schemas"]["ApprovalBundleDecision"];
export type BundleOutcome =
  components["schemas"]["ApprovalBundleMember"]["outcome"];

// A page tops out at 50, and the pending/decided partition below needs the
// whole set to sort and filter correctly. So this walks every page by
// `next_cursor` before the query resolves. A surface that only needs how many
// are waiting reads usePendingApprovalCount instead.
async function fetchAllApprovals(
  status: ApprovalStatus,
  target?: { entityType?: string; entityId?: string; bundleId?: string },
): Promise<Approval[]> {
  const all: Approval[] = [];
  let cursor: string | null | undefined;
  do {
    const data = unwrap(
      await api.GET("/approvals", {
        params: {
          query: {
            status,
            limit: 50,
            cursor: cursor ?? undefined,
            bundle_id: target?.bundleId,
            target_entity_type: target?.entityType,
            target_entity_id: target?.entityId,
          },
        },
      }),
    );
    all.push(...data.data);
    cursor = data.page?.has_more ? (data.page.next_cursor ?? null) : null;
  } while (cursor);
  return all;
}

// Refetching on focus overrides the app-wide default (main.tsx turns it off)
// because approvals go stale on their own: the server computes expiry lazily at
// read time, so a row that ages out while the tab sits in the background is
// still counted as waiting by the rail badge until something asks again. Coming
// back to the tab is exactly the moment the count has to be true, and it costs
// one small request rather than a background poll.
export function useApprovals(status: ApprovalStatus, enabled = true) {
  return useQuery({
    queryKey: ["approvals", status],
    enabled,
    ...(!isLiveWorkRead(["approvals", status])
      ? { refetchOnWindowFocus: true }
      : {}),
    queryFn: async (): Promise<ApprovalPage> => ({
      data: await fetchAllApprovals(status),
    }),
  });
}

export type PendingApprovalCount =
  components["schemas"]["ApprovalPendingCount"];

// Under ["approvals", "pending"], so every approval mutation and every pending
// invalidation refreshes it, and the client treats it as a live read.
export const pendingApprovalCountKey = ["approvals", "pending", "count"];

// The longest delay setTimeout honours; a longer one fires at once.
const MAX_TIMER_MS = 2_147_483_647;

// The server still counts a row AT its expiry instant, so read just after it.
const PAST_EXPIRY_MS = 1_000;

/** How long until the count drops by itself, measured on the server's clock. */
export function untilNextExpiry(count: PendingApprovalCount): number | null {
  if (!count.next_expiry_at) return null;
  const wait = Date.parse(count.next_expiry_at) - Date.parse(count.as_of);
  return Math.min(Math.max(wait, 0) + PAST_EXPIRY_MS, MAX_TIMER_MS);
}

// How many approvals wait for the reader, counted by the server over the rows
// the Pending tab lists. An expiry lowers the number with no event, so the
// hook reads it again when the first counted approval expires.
export function usePendingApprovalCount() {
  const query = useQuery({
    queryKey: pendingApprovalCountKey,
    queryFn: async (): Promise<PendingApprovalCount> =>
      unwrap(await api.GET("/approvals/pending-count")),
  });
  const { data, refetch } = query;
  useEffect(() => {
    const wait = data ? untilNextExpiry(data) : null;
    if (wait === null) return;
    const timer = setTimeout(() => void refetch(), wait);
    return () => clearTimeout(timer);
  }, [data, refetch]);
  return query;
}

// The Pending tab: status=pending, but a lazily-expired row (wire
// status="expired") is DROPPED — it is not actionable, it belongs in
// Decided (AC-7: expired never listed as pending).
export function usePendingApprovals(): QueryLike<ApprovalPage> {
  const pending = useApprovals("pending");
  return {
    isPending: pending.isPending,
    isError: pending.isError,
    error: pending.error,
    data: pending.data
      ? { data: pending.data.data.filter((a) => a.status !== "expired") }
      : undefined,
    refetch: pending.refetch,
  };
}

// decided_at is the honest sort key (when the human actually acted); an
// expired item auto-transitioned with no decided_at, so it falls back to
// expires_at — never created_at, which would misrepresent an old-but-just-
// expired item as freshly decided.
function decidedRank(approval: Approval): number {
  const at = approval.decided_at ?? approval.expires_at ?? approval.created_at;
  return at ? new Date(at).getTime() : 0;
}

// The Decided tab: approved + rejected from their own typed status queries;
// EXPIRED items (unqueryable) salvaged from the status=pending response.
// Merge, newest decision first. The two decided-only fetches are gated so
// they don't fire on the default Pending view; the pending query is always-on
// (shared cache key with usePendingApprovals) and supplies the expired rows.
export function useDecidedApprovals(enabled = true): QueryLike<ApprovalPage> {
  const approved = useApprovals("approved", enabled);
  const rejected = useApprovals("rejected", enabled);
  const pending = useApprovals("pending");
  const all = [approved, rejected, pending];
  const isPending = all.some((query) => query.isPending);
  const isError = all.some((query) => query.isError);
  const firstError = all.find((query) => query.isError)?.error ?? null;
  const expired = (pending.data?.data ?? []).filter(
    (a) => a.status === "expired",
  );
  const data: ApprovalPage | undefined =
    isPending || isError
      ? undefined
      : {
          data: [
            ...(approved.data?.data ?? []),
            ...(rejected.data?.data ?? []),
            ...expired,
          ].sort((a, b) => decidedRank(b) - decidedRank(a)),
        };
  const refetch = () => {
    approved.refetch();
    rejected.refetch();
    pending.refetch();
  };
  return { isPending, isError, error: firstError, data, refetch };
}

// What is staged against ONE record. A record page showing its own queue must
// read the queue itself: the composite 360 carries a capped first page of
// approvals for the count, and deciding through that page would leave the rest
// reachable only through the workspace-wide Decisions surface, which is not the
// queue the reader was working.
export function useTargetApprovals(
  entityType: string,
  entityId: string,
  enabled = true,
) {
  return useQuery({
    queryKey: ["approvals", "pending", entityType, entityId],
    enabled,
    queryFn: async (): Promise<ApprovalPage> => ({
      data: (
        await fetchAllApprovals("pending", { entityType, entityId })
      ).filter((approval) => approval.status === "pending"),
    }),
  });
}

export function useBundleApprovals(bundleId: string) {
  return useQuery({
    queryKey: ["approvals", "pending", "bundle", bundleId],
    queryFn: async (): Promise<ApprovalPage> => ({
      data: (await fetchAllApprovals("pending", { bundleId })).filter(
        (approval) => approval.status === "pending",
      ),
    }),
  });
}

export function invalidateApprovalViews(client: QueryClient) {
  return Promise.all(
    [
      ["approvals"],
      ["approval"],
      worklistKey,
      briefKey,
      magicKey,
      digestKey,
    ].map((queryKey) => client.invalidateQueries({ queryKey })),
  );
}
