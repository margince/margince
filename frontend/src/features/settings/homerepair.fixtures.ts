// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";

export const repairReview: components["schemas"]["HomeRepairReview"] = {
  as_of: "2026-04-01T12:00:00Z",
  digest: "a".repeat(64),
  rule_version: "test",
  limit: 2,
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
