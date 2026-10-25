// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";

// The records the contact page galleries share: the page, its rail and its
// overview read the same contact, so a change shows in all three.

export type View = components["schemas"]["Contact360"];

export const page = { has_more: false, next_cursor: null };

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
// allowed. The demo-seed spirit: a record with enough on it to fill every
// card, and nothing invented past what the fixture states.
export const populated: View = {
  as_of: "2026-08-13T09:00:00Z",
  contact: {
    id: "p-1",
    full_name: "Dana Buyer",
    first_name: "Dana",
    last_name: "Buyer",
    title: "Head of Fleet",
    owner_id: "u-1",
    visibility: "owner",
    writable: true,
    version: 7,
    social: { linkedin: "https://linkedin.com/in/danabuyer" },
    address: { city: "Munich", country: "DE" },
    primary_email: "dana@brandt.example",
    emails: [
      {
        id: "pe-1",
        contact_id: "p-1",
        email: "dana@brandt.example",
        email_type: "work",
        is_primary: true,
        position: 0,
        source: "manual",
        captured_by: "human:u1",
      },
    ],
    phones: [
      {
        id: "pp-1",
        contact_id: "p-1",
        phone: "+493012345678",
        phone_type: "work",
        is_primary: true,
        position: 0,
        source: "manual",
        captured_by: "human:u1",
      },
    ],
    source: "manual",
    captured_by: "human:u1",
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-08-01T08:00:00Z",
  },
  last_inbound_at: "2026-08-01T10:15:00Z",
  last_outbound_at: "2026-07-20T09:00:00Z",
  sections_omitted: [],
  network: {
    colleagues: [
      {
        user_id: "u-2",
        display_name: "Sam Rivera",
        strength: 0.7,
        strength_bucket: "strong",
        interactions_90d: 6,
        last_at: "2026-07-30T09:00:00Z",
        inbound_90d: 3,
        outbound_90d: 3,
        last_inbound_at: "2026-07-30T09:00:00Z",
        last_outbound_at: "2026-07-29T09:00:00Z",
      },
    ],
  },
  employments: {
    data: [
      {
        relationship_id: "rel-1",
        company_id: "o-1",
        company_name: "Brandt Automotive GmbH",
        role: "Head of Fleet",
        is_current_primary: true,
        started_at: "2022-03-01T00:00:00Z",
        ended_at: null,
      },
    ],
    page,
  },
  activities: {
    data: [
      {
        occurred_precision: "instant",
        id: "a-1",
        kind: "email",
        direction: "inbound",
        subject: "Re: retrofit timeline",
        body: "Can we push the fleet retrofit review back a week?",
        occurred_at: "2026-08-01T10:15:00Z",
        links: [{ entity_type: "contact", entity_id: "p-1" }],
        source: "gmail",
        captured_by: "connector:gmail",
        created_at: "2026-08-01T10:15:00Z",
        updated_at: "2026-08-01T10:15:00Z",
        is_done: false,
      },
    ],
    page,
  },
  commercial: {
    role: "champion",
    committee: [],
  },
  // The server sends this section whenever the grant admits it, empty rows
  // and all. A fixture without it is a payload no permitted reader receives,
  // and the Deals tab would read as "could not be loaded".
  deal_roles: {
    data: [
      {
        relationship_id: "r-1",
        deal_id: "d-1",
        deal_title: "Fleet retrofit, 40 vehicles",
        deal_stage: "Proposal",
        role: "champion",
      },
    ],
    page: { has_more: false },
  },
  next_meeting: {
    activity_id: "a-2",
    starts_at: "2026-08-20T13:00:00Z",
    occurred_precision: "instant",
    subject: "Fleet retrofit walkthrough",
    linked_deal_id: null,
    participants: [{ contact_id: "p-1", full_name: "Dana Buyer" }],
  },
  claims: [
    {
      id: "c-1",
      kind: "commitment_ours",
      body: "send the updated retrofit quote",
      source_activity_id: "a-1",
      occurred_precision: "instant",
      source_quote: "Can we push the fleet retrofit review back a week?",
      source_label: "Re: retrofit timeline",
      occurred_at: "2026-08-01T10:15:00Z",
      status: "open",
      due_at: "2026-08-15T00:00:00Z",
      needs_review: false,
    },
    {
      id: "c-2",
      kind: "priority",
      body: "keep the depot offline window under four hours",
      source_activity_id: "a-1",
      occurred_precision: "instant",
      source_quote: "We can't have the depot down for more than four hours.",
      source_label: "Re: retrofit timeline",
      occurred_at: "2026-08-01T10:15:00Z",
      status: "open",
      needs_review: false,
    },
  ],
  conversation_memory: [
    {
      key: "thread-1",
      channel: "email",
      direction: "inbound",
      title: "Re: retrofit timeline",
      summary: "Dana asked to push the retrofit review back a week.",
      generated_by: "deterministic",
      occurred_at: "2026-08-01T10:15:00Z",
      activity_count: 3,
      status: "unanswered",
      linked_deal_id: null,
      first_activity_id: "a-1",
    },
  ],
  since_last_visit: {
    baseline_at: "2026-07-25T09:00:00Z",
    new_activities: 1,
  },
  moment: meetingPrepMoment,
};

// The same record read by someone whose grant covers none of the relationship
// sections. Every reading says so, instead of reading as a thin or dormant
// contact. No seeded demo grant reaches this state, so this fixture is the
// only place it renders.
export const withheld: View = {
  ...populated,
  deal_roles: undefined,
  last_inbound_at: undefined,
  last_outbound_at: undefined,
  activities: undefined,
  commercial: undefined,
  next_meeting: undefined,
  sections_omitted: [
    "last_touch",
    "activities",
    "deal_roles",
    "commercial",
    "next_meeting",
    "consent",
  ],
};

// The contact a chat connector creates: no address, no number, and two channel
// identities (one still deliverable, one not). The Consent & Channels block
// must report the two identities it has, not the two transports it lacks.
export const channelReached: View = {
  ...populated,
  contact: {
    ...populated.contact,
    emails: [],
    primary_email: null,
    phones: [],
    reachability: [
      {
        provider: "zalo_oa",
        reachable: true,
        since: "2026-08-01T09:00:00Z",
      },
      {
        provider: "dispact",
        reachable: false,
        since: "2026-08-09T09:00:00Z",
      },
    ],
  },
};

// The same contact with the conversation a reply would continue: the whole
// relationship arrived over a chat channel. The page's lead verb reads it to
// name its transport, and so do two cards further down. Neither may name it
// mail (an envelope on the memory row, "Email thread" under the brief).
export const chatConversation: View = {
  ...channelReached,
  activities: {
    data: [
      {
        occurred_precision: "instant",
        id: "a-9",
        kind: "message",
        channel_provider: "zalo_oa",
        direction: "inbound",
        subject: null,
        body: "Bên mình cần báo giá cho 40 xe.",
        occurred_at: "2026-08-12T04:20:00Z",
        links: [{ entity_type: "contact", entity_id: "p-1" }],
        source: "ext:zalo-oa:zalo",
        captured_by: "connector:zalo-oa",
        created_at: "2026-08-12T04:20:00Z",
        updated_at: "2026-08-12T04:20:00Z",
        is_done: false,
      },
    ],
    page,
  },
  conversation_memory: [],
};
