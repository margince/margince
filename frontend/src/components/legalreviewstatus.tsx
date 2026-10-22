// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Badge } from "../design-system/atoms";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { LegalReviewSummary } from "../lib/legalreviews.queries";

type Status = LegalReviewSummary["status"];

// Keyed by the generated union, so a status added to crm.yaml fails the typecheck here.
const STATUS_LABEL: Readonly<Record<Status, MessageKey>> = {
  requested: "legalReview.status.requested",
  approved: "legalReview.status.approved",
  changes_needed: "legalReview.status.changes_needed",
  withdrawn: "legalReview.status.withdrawn",
};

const STATUS_TONE: Readonly<
  Record<Status, "info" | "success" | "warning" | "default">
> = {
  requested: "info",
  approved: "success",
  changes_needed: "warning",
  withdrawn: "default",
};

/** Where a legal review stands, as the deal page and the queue both show it. */
export function LegalReviewStatusBadge({
  status,
}: Readonly<{ status: Status }>) {
  const t = useT();
  return <Badge tone={STATUS_TONE[status]}>{t(STATUS_LABEL[status])}</Badge>;
}
