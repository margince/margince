// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { skipToken, useQuery, useQueryClient } from "@tanstack/react-query";
import type { Receipt } from "../../lib/handled";
import type { MarginceChangeBundle } from "../../lib/marginchanges";
import { receiptAuditId } from "../../lib/receiptreview";
import type { DealChanges } from "./brief.doneforyou.groups";

const REVIEW_KEY = ["done-for-you-review"];
export const REVIEW_PARAM = "review_deals";

type Review = Readonly<{
  groups: readonly Readonly<{ id: string; audits: readonly string[] }>[];
  bundles: readonly MarginceChangeBundle[];
  selected: readonly string[];
  loaded: number;
}>;

export function useDealReview() {
  const client = useQueryClient();
  const read = useQuery<Review>({
    queryKey: REVIEW_KEY,
    queryFn: skipToken,
    enabled: false,
    gcTime: Number.POSITIVE_INFINITY,
  });
  return {
    data: read.data,
    keep: (review: Review) => client.setQueryData(REVIEW_KEY, review),
    update: (change: (review: Review) => Review, initial: Review) =>
      client.setQueryData<Review>(REVIEW_KEY, (review) =>
        change(review ?? initial),
      ),
  };
}

export function reviewedDeals(
  groups: readonly DealChanges[],
  bundles: readonly MarginceChangeBundle[],
  allBundles: readonly MarginceChangeBundle[],
  loaded: number,
): Review {
  return {
    groups: groups.map((group) => ({
      id: group.dealId,
      audits: [
        ...new Set([
          ...group.receipts.map(
            (receipt) => receiptAuditId(receipt) ?? receipt.id,
          ),
          ...(allBundles.find((bundle) => bundle.deal_id === group.dealId)
            ?.audit_log_ids ?? []),
        ]),
      ],
    })),
    // Keep references and versions; names and evidence come from the current read.
    bundles: bundles.map((bundle) => ({
      deal_id: bundle.deal_id,
      version: bundle.version,
      audit_log_ids: bundle.audit_log_ids,
      kinds: bundle.kinds,
      latest_at: bundle.latest_at,
    })),
    selected: bundles.map((bundle) => bundle.deal_id),
    loaded,
  };
}

export function reviewGroups(
  review: Review | undefined,
  groups: readonly DealChanges[],
  receipts: readonly Receipt[],
): DealChanges[] {
  return (review?.groups ?? []).flatMap((saved) => {
    const current = groups.find((group) => group.dealId === saved.id);
    const audits = new Set(saved.audits);
    const rows = receipts.filter(
      (receipt) =>
        receipt.subject?.type === "deal" &&
        receipt.subject.id === saved.id &&
        audits.has(receiptAuditId(receipt) ?? receipt.id),
    );
    if (!current && !rows.some((row) => row.subject?.label)) return [];
    return [
      {
        dealId: saved.id,
        name: current?.name ?? rows[0]?.subject?.label ?? undefined,
        receipts: rows,
        unloaded: Math.max(0, saved.audits.length - rows.length),
      },
    ];
  });
}
