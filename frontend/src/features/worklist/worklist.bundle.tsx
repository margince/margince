// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useNow } from "../../format/now";
import { type Approval, useBundleApprovals } from "../../lib/approvals.queries";
import { deckItems } from "../../lib/brief.decisions.items";
import { DecisionsSection } from "../brief/brief.decisions";

export function ApprovalBundleReview({
  approval,
}: Readonly<{ approval: Approval }>) {
  const pending = useBundleApprovals(approval.bundle_id ?? "");
  const now = useNow(60_000);
  const members =
    pending.data?.data.filter(
      (entry) =>
        entry.bundle_id === approval.bundle_id && entry.status === "pending",
    ) ?? [];
  return (
    <DecisionsSection
      expanded
      items={deckItems(members)}
      nowMs={now}
      state={
        pending.isPending ? "loading" : pending.isError ? "failed" : "ready"
      }
      onAlreadyDecided={() => void pending.refetch()}
    />
  );
}
