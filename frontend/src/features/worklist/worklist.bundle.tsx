// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useNow } from "../../format/now";
import {
  type Approval,
  useBundleApprovals,
} from "../automations/approvals.queries";
import { DecisionsSection } from "../brief/brief.decisions";
import { deckItems } from "../brief/brief.decisions.items";

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
