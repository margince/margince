// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Receipt } from "./worklist.queries";

export const automaticStageReceipt: Receipt = {
  id: "01a00000-0000-7000-8000-000000000010",
  kind: "stage_progression",
  summary:
    "Moved PIM Rollout from Discovery to Proposal — the customer confirmed the scope and requested a quote.",
  occurred_at: "2026-09-13T07:15:00Z",
  subject: {
    type: "deal",
    id: "01a00000-0000-7000-8000-000000000011",
    label: "PIM Rollout",
  },
  review: {
    kind: "stage",
    accepted: false,
    reversed: false,
    version: 7,
    can_undo: true,
    can_accept: true,
    writable: true,
  },
};

export const automaticDateReceipt: Receipt = {
  ...automaticStageReceipt,
  id: "01a00000-0000-7000-8000-000000000012",
  kind: "close_date_correction",
  summary:
    "Changed the close date on PIM Rollout: 2026-09-01 → 2026-09-27 — the previous date passed without a signed agreement.",
  undo: {
    audit_log_id: "01a00000-0000-7000-8000-000000000012",
    version: 7,
    reversed: false,
  },
  review: {
    kind: "close_date",
    accepted: false,
    reversed: false,
    version: 7,
    can_undo: true,
    can_accept: true,
    writable: true,
  },
};
