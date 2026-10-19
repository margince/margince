// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MagicLine, MagicReceipt } from "./magic.queries";

// A night as the receipt reports one: a mailbox sync filing in bulk after
// midnight, readers enriching what it filed, agents booking and moving work
// towards morning, two proposals waiting for a word and one job that broke.
// The stories draw it; the window is an evening-to-morning brief.

const NOT_UNDOABLE = {
  undoable: false,
  reason: "not_a_replayable_verb",
} as const;

function undoable(serial: string): MagicLine["undo"] {
  return {
    undoable: true,
    audit_id: `00000000-0000-7000-8000-0000000b00${serial}`,
    version: 4,
  };
}

function record(type: string, serial: string, label: string) {
  return { type, id: `00000000-0000-7000-8000-0000000a00${serial}`, label };
}

const agent = (key: string): MagicLine["actor"] => ({
  type: "agent",
  id: "overnight",
  label: { key },
});

const job = (key: string): MagicLine["actor"] => ({
  type: "system",
  id: key,
  label: { key },
});

export const BUSY_NIGHT: MagicReceipt = {
  as_of: "2026-09-13T08:05:00Z",
  since: "2026-09-12T15:40:00Z",
  done: [
    {
      id: "00000000-0000-7000-8000-000000000101",
      occurred_at: "2026-09-13T04:40:00Z",
      lane: "done",
      summary: { key: "magic.action.send_email" },
      consequence: "magic.consequence.message_sent",
      entity: record("contact", "01", "Mira Scholz"),
      undo: { undoable: false, reason: "not_a_replayable_verb" },
      actor: agent("magic.by.overnight_agent"),
    },
    {
      id: "00000000-0000-7000-8000-000000000102",
      occurred_at: "2026-09-13T04:30:00Z",
      lane: "done",
      summary: { key: "magic.action.advance_stage" },
      consequence: "magic.consequence.stage_moved",
      entity: record("deal", "02", "PIM Rollout"),
      undo: undoable("02"),
      actor: agent("magic.by.overnight_agent"),
    },
    {
      id: "00000000-0000-7000-8000-000000000103",
      occurred_at: "2026-09-13T01:20:00Z",
      lane: "done",
      summary: { key: "magic.action.company_profile_read" },
      reason: { key: "magic.why.site_read_each" },
      entity: record("company", "03", "Aster Handel"),
      undo: NOT_UNDOABLE,
      actor: job("magic.by.website_reader"),
      count: 3,
    },
    {
      id: "00000000-0000-7000-8000-000000000104",
      occurred_at: "2026-09-13T01:10:00Z",
      lane: "done",
      summary: {
        key: "magic.action.fields_changed",
        values: { fields: "job title, phone" },
      },
      reason: { key: "magic.why.signature" },
      entity: record("contact", "04", "Jonas Brandt"),
      undo: NOT_UNDOABLE,
      actor: job("magic.by.signature_reader"),
      count: 5,
    },
    {
      id: "00000000-0000-7000-8000-000000000105",
      occurred_at: "2026-09-13T01:00:00Z",
      lane: "done",
      summary: { key: "magic.action.mail_filed" },
      reason: { key: "magic.why.mail_filed" },
      entity: record("contact", "05", "Anna Keller"),
      undo: NOT_UNDOABLE,
      actor: job("magic.by.mail_filing"),
      count: 42,
    },
    {
      id: "00000000-0000-7000-8000-000000000106",
      occurred_at: "2026-09-12T16:05:00Z",
      lane: "done",
      summary: { key: "magic.action.schedule" },
      consequence: "magic.consequence.meeting_booked",
      entity: record("deal", "06", "Fleet retrofit"),
      undo: undoable("06"),
      actor: agent("magic.by.overnight_agent"),
    },
  ],
  needs_you: [
    {
      id: "00000000-0000-7000-8000-000000000107",
      occurred_at: "2026-09-13T03:55:00Z",
      lane: "needs_you",
      summary: {
        key: "magic.action.approval_advance_deal",
        values: { target: "Depot rollout" },
      },
      consequence: "magic.consequence.awaits_your_decision",
      actor: agent("magic.by.overnight_agent"),
    },
    {
      id: "00000000-0000-7000-8000-000000000108",
      occurred_at: "2026-09-13T03:50:00Z",
      lane: "needs_you",
      summary: {
        key: "magic.action.approval_send_email",
        values: { target: "Anna Weber" },
      },
      consequence: "magic.consequence.awaits_your_decision",
      actor: agent("magic.by.overnight_agent"),
    },
  ],
  could_not_complete: [
    {
      id: "00000000-0000-7000-8000-000000000109",
      occurred_at: "2026-09-13T05:02:00Z",
      lane: "could_not_complete",
      summary: {
        key: "magic.action.automation_troubled",
        values: { name: "Nightly follow-up", outcome: "timed out" },
      },
      consequence: "magic.consequence.automation_did_nothing",
      undo: { undoable: false, reason: "no_completed_change" },
      actor: job("magic.by.automation"),
    },
  ],
  watching: [],
  totals: { done: 6, needs_you: 2, could_not_complete: 1, watching: 0 },
  not_shown: [{ reason: "unadmitted_action", count: 12 }],
  sources_unavailable: [],
};
