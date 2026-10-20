// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";

export const repairReview: components["schemas"]["HomeRepairReview"] = {
  as_of: "2026-04-01T12:00:00Z",
  digest: "a".repeat(64),
  rule_version: "test",
  limit: 2,
  counts: { archive: 1, withdraw: 0, review: 1, keep: 0 },
  tasks: [
    {
      id: "00000000-0000-0000-0000-000000000001",
      version: 1,
      action: "archive_automatic_reminder",
      reason: "historical_without_current_deadline",
    },
    {
      id: "00000000-0000-0000-0000-000000000002",
      version: 2,
      action: "human_review",
      reason: "human_adoption_not_excluded",
    },
  ],
};

export const proposalRepairReview: components["schemas"]["HomeRepairReview"] = {
  ...repairReview,
  limit: 1,
  counts: { archive: 0, withdraw: 1, review: 0, keep: 0 },
  tasks: [
    {
      id: repairReview.tasks[0].id,
      source: "approval",
      snapshot: "b".repeat(64),
      version: 0,
      action: "withdraw_proposal",
      reason: "duplicate_pending_proposal",
    },
  ],
};
