// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

// The nine rungs of the contact page's lead moment, one fixture each, for the
// page gallery beside this file. They are what ContactToday renders and what
// the page stories spread into a record; nothing here is a story.

// The lead moment on the meeting-prep rung: a booked meeting close enough to
// need a brief. Typed on its own so the story rendering ContactToday directly
// never has to narrow an optional field back out of the 360.
export const meetingPrepMoment: components["schemas"]["ContactMoment"] = {
  claim_key: "meeting_prep:p-1:a-2",
  evidence_fingerprint: "fp-meeting-1",
  rule: "meeting_prep",
  rule_version: "v1",
  headline: "Dana's retrofit walkthrough is in 7 days.",
  why_now: "A booked meeting inside two weeks with no brief prepared yet.",
  confidence: "observed_fact",
  freshness_at: "2026-08-13T09:00:00Z",
  evidence: [
    {
      type: "activity",
      id: "a-2",
      label: "Fleet retrofit walkthrough, 20 Aug",
      observed_at: "2026-08-20T13:00:00Z",
    },
  ],
  recommended_action: {
    kind: "open_meeting_brief",
    label: "Open meeting brief",
    destination: { surface: "meeting_brief" },
    state: "available",
  },
  secondary_actions: [
    {
      kind: "draft_reply",
      label: "Confirm the time",
      state: "available",
    },
  ],
};

// One contact at a company: one unanswered inbound thread, a meeting
// accepted, no open deal, one colleague who knows them, email consent
// allowed. The demo-seed spirit — a record with enough on it to fill every
// card, and nothing invented past what the fixture states.
// The same record with a moment on the amber ladder rung: a relationship that
// stopped rather than one that is merely upcoming, so both tints of the lead
// card are on screen across the gallery.
export const goneQuietMoment: components["schemas"]["ContactMoment"] = {
  claim_key: "gone_quiet:p-1",
  evidence_fingerprint: "fp-quiet-1",
  rule: "gone_quiet",
  rule_version: "v1",
  headline: "Dana has gone quiet for 18 days.",
  why_now:
    "No reply in 18 days after two outbound messages — the gone-quiet rung fired ahead of meeting prep.",
  confidence: "observed_fact",
  freshness_at: "2026-08-13T09:00:00Z",
  evidence: [
    {
      type: "activity",
      id: "a-1",
      label: "Re: retrofit timeline",
      snippet: "Can we push the fleet retrofit review back a week?",
      observed_at: "2026-08-01T10:15:00Z",
    },
  ],
  recommended_action: {
    kind: "draft_reply",
    label: "Send a check-in",
    state: "available",
  },
  secondary_actions: [
    {
      kind: "ask_colleague",
      label: "Ask Sam Rivera",
      destination: { surface: "record", entity_id: "u-2" },
      state: "available",
    },
  ],
};

// The seven rungs of the ladder LeadMoment/LeadMomentWarning above never
// reach: ContactToday renders one ContactMomentRule per fixture, and a gallery
// with only meeting_prep and gone_quiet on screen hides the other seven the
// component can render. Spread across them: two evidence items (the
// "sources" plural), a `will_confirm` action, a `blocked` one with its
// reason, and a freshness older than the moment's own headline date.

export const reEngagedMoment: components["schemas"]["ContactMoment"] = {
  claim_key: "re_engaged:p-1",
  evidence_fingerprint: "fp-reengaged-1",
  rule: "re_engaged",
  rule_version: "v1",
  headline: "Dana wrote back after six weeks quiet.",
  why_now: "A reply landed after a long silence, the door is open again.",
  confidence: "observed_fact",
  freshness_at: "2026-08-13T09:00:00Z",
  evidence: [
    {
      type: "activity",
      id: "a-3",
      label: "Re: still interested in the retrofit",
      observed_at: "2026-08-13T08:00:00Z",
    },
    {
      type: "activity",
      id: "a-1",
      label: "Re: retrofit timeline",
      observed_at: "2026-08-01T10:15:00Z",
    },
  ],
  recommended_action: {
    kind: "draft_reply",
    label: "Welcome her back",
    state: "will_confirm",
  },
  secondary_actions: [
    { kind: "schedule_meeting", label: "Book a catch-up", state: "available" },
  ],
};

export const jobChangeMoment: components["schemas"]["ContactMoment"] = {
  claim_key: "job_change:p-1",
  evidence_fingerprint: "fp-jobchange-1",
  rule: "job_change",
  rule_version: "v1",
  headline: "Dana moved to Head of Fleet at Brandt Automotive.",
  why_now: "A recorded employment change: the buying context here just moved.",
  confidence: "observed_fact",
  // Older than the headline's own evidence, so the "updated N days ago" wording
  // reads as a real gap rather than same-day freshness.
  freshness_at: "2026-08-05T09:00:00Z",
  evidence: [
    {
      type: "relationship_change",
      label: "Employment updated: Head of Fleet at Brandt Automotive GmbH",
      observed_at: "2026-08-05T09:00:00Z",
    },
  ],
  recommended_action: {
    kind: "open_record",
    label: "Review the account",
    destination: { surface: "record", entity_type: "company" },
    state: "available",
  },
  secondary_actions: [
    {
      kind: "draft_reply",
      label: "Congratulate her",
      state: "blocked",
      blocked_reason: "No consent recorded for marketing outreach yet.",
    },
  ],
};

export const overduePromiseMoment: components["schemas"]["ContactMoment"] = {
  claim_key: "overdue_promise:p-1:c-1",
  evidence_fingerprint: "fp-overdue-1",
  rule: "overdue_promise",
  rule_version: "v1",
  headline: "The updated retrofit quote is three days late.",
  why_now: "A commitment past its own due date, still open.",
  confidence: "observed_fact",
  freshness_at: "2026-08-13T09:00:00Z",
  evidence: [
    {
      type: "activity",
      id: "a-1",
      label: "Re: retrofit timeline",
      snippet: "Can we push the fleet retrofit review back a week?",
      observed_at: "2026-08-01T10:15:00Z",
    },
  ],
  recommended_action: {
    kind: "draft_reply",
    label: "Send the quote",
    state: "available",
  },
};

export const publicSignalMoment: components["schemas"]["ContactMoment"] = {
  claim_key: "public_signal:p-1",
  evidence_fingerprint: "fp-publicsignal-1",
  rule: "public_signal",
  rule_version: "v1",
  headline: "Dana posted about the depot's EV rollout timeline.",
  why_now: "A public statement that bears on the retrofit conversation.",
  confidence: "medium",
  freshness_at: "2026-08-11T09:00:00Z",
  evidence: [
    {
      type: "activity",
      label: 'LinkedIn post: "2027 is the depot\'s EV deadline"',
      observed_at: "2026-08-11T09:00:00Z",
    },
  ],
  recommended_action: {
    kind: "draft_reply",
    label: "Reference it in your next note",
    state: "available",
  },
};

export const missingNextStepMoment: components["schemas"]["ContactMoment"] = {
  claim_key: "missing_next_step:p-1",
  evidence_fingerprint: "fp-missingstep-1",
  rule: "missing_next_step",
  rule_version: "v1",
  headline: "The retrofit walkthrough has no next step booked after it.",
  why_now: "A meeting is on the calendar with nothing recorded for after it.",
  confidence: "observed_fact",
  freshness_at: "2026-08-13T09:00:00Z",
  evidence: [
    {
      type: "task",
      label: "Fleet retrofit walkthrough, 20 Aug",
      observed_at: "2026-08-20T13:00:00Z",
    },
  ],
  recommended_action: {
    kind: "schedule_meeting",
    label: "Book the follow-up",
    state: "available",
  },
};

export const thinRelationshipMoment: components["schemas"]["ContactMoment"] = {
  claim_key: "thin_relationship:p-1",
  evidence_fingerprint: "fp-empty",
  rule: "thin_relationship",
  rule_version: "v1",
  headline: "No interactions recorded",
  why_now:
    "No interactions or colleague connections were found in the records available to you.",
  confidence: "observed_fact",
  evidence: [],
  recommended_action: {
    kind: "log_activity",
    label: "Log an interaction",
    state: "available",
  },
};

// Rung 10, the quiet-success case: nothing needs the reader today, and it
// renders through this same component rather than an empty card
export const nothingNeededMoment: components["schemas"]["ContactMoment"] = {
  claim_key: "nothing_needed:p-1",
  evidence_fingerprint: "fp-nothingneeded-1",
  rule: "nothing_needed",
  rule_version: "v1",
  headline: "Nothing needs you on this account today.",
  why_now: "Every open loop is answered and the next meeting is booked.",
  confidence: "observed_fact",
  freshness_at: "2026-08-13T09:00:00Z",
  evidence: [
    {
      type: "activity",
      id: "a-2",
      label: "Fleet retrofit walkthrough, 20 Aug",
      observed_at: "2026-08-20T13:00:00Z",
    },
  ],
  recommended_action: {
    kind: "open_record",
    label: "Open the record",
    destination: {
      surface: "record",
      entity_type: "contact",
      entity_id: "p-1",
    },
    state: "available",
  },
};
