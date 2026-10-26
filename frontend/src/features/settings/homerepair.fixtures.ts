// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";

const reviewedTaskID = "00000000-0000-0000-0000-000000000001";

export const repairReview: components["schemas"]["HomeRepairReview"] = {
  as_of: "2026-04-01T12:00:00Z",
  digest: "a".repeat(64),
  rule_version: "test",
  limit: 2,
  counts: {
    reclassify: 0,
    archive: 1,
    withdraw: 0,
    supersede: 0,
    review: 1,
    keep: 0,
  },
  tasks: [
    {
      id: reviewedTaskID,
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
  counts: {
    reclassify: 0,
    archive: 0,
    withdraw: 1,
    supersede: 0,
    review: 0,
    keep: 0,
  },
  tasks: [
    {
      id: reviewedTaskID,
      source: "approval",
      snapshot: "b".repeat(64),
      version: 0,
      action: "withdraw_proposal",
      reason: "duplicate_pending_proposal",
    },
  ],
};

export const suggestionRepairReview: components["schemas"]["HomeRepairReview"] =
  {
    ...repairReview,
    limit: 1,
    counts: {
      reclassify: 0,
      archive: 0,
      withdraw: 0,
      supersede: 1,
      review: 0,
      keep: 0,
    },
    tasks: [
      {
        id: "01a0e9f1-0000-7000-8000-000000000001",
        source: "suggestion",
        version: 1,
        action: "supersede_suggestion",
        reason: "buyer_no_longer_verified",
      },
    ],
  };

export const classificationRepairReview: components["schemas"]["HomeRepairReview"] =
  {
    ...repairReview,
    limit: 1,
    preview_token: "r".repeat(43),
    counts: {
      reclassify: 1,
      archive: 0,
      withdraw: 0,
      supersede: 0,
      review: 0,
      keep: 0,
    },
    tasks: [
      {
        id: reviewedTaskID,
        source: "verdict",
        version: 1,
        snapshot: "c".repeat(64),
        action: "reclassify_message",
        reason: "classification_changed",
        before_verdict: "asks_us",
        after_verdict: "informs_us",
      },
    ],
  };

export const settlementRepairReview: components["schemas"]["HomeRepairReview"] =
  {
    ...repairReview,
    limit: 2,
    counts: {
      reclassify: 0,
      archive: 0,
      withdraw: 0,
      supersede: 0,
      review: 0,
      keep: 1,
      revalidate: 1,
    },
    tasks: [
      {
        id: reviewedTaskID,
        source: "settlement",
        version: 3,
        snapshot: "d".repeat(64),
        action: "revalidate_settlement",
        reason: "settlement_revalidate",
      },
      {
        id: "0192aaaa-0000-7000-8000-00000000beef",
        source: "settlement",
        version: 2,
        snapshot: "e".repeat(64),
        action: "leave_current",
        reason: "settlement_human_work",
      },
    ],
  };
