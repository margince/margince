// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { Deal, MorningBrief, MorningDigest } from "./home.queries";
import { jsonResponse } from "./story-utils";

// The fixtures every Home story is built from — the whole page in
// `home.stories.tsx`, its parts one at a time in `home.parts.stories.tsx`. They
// live in their own module because the two catalogs document the SAME morning:
// a second copy of these proposals would let the parts drift from the page they
// are parts of, one edited fixture at a time.
//
// EVERY INSTANT IS FIXED. A fixture built with `new Date()` documents whatever
// day the catalog was opened on, and the two things on this page that read a
// clock — the greeting band and a proposal's expiry — would then say something
// different every time somebody looked. Expiries are therefore either ABSENT
// (calm, and stable forever) or a fixed instant in the past (the lapsed
// proposal, which stays lapsed).

export type Approval = components["schemas"]["Approval"];

// ── Fixtures ────────────────────────────────────────────────────────────────

/** One staged proposal, named by the sentence its card leads with. */
export function proposal(
  id: string,
  summary: string,
  over: Partial<Approval> = {},
): Approval {
  return {
    id,
    kind: "send_email",
    status: "pending",
    proposed_by: "agent:runner",
    summary,
    proposed_change: {
      subject: "Re: the two dates that work",
      body: "Hi Anna — following up on the kickoff. Either Tuesday or Thursday works on our side; shall I hold Tuesday 10:00?",
    },
    confidence: 0.62,
    evidence: [
      {
        evidence_snippet: "…shall we sync next week?…",
        source_type: "activity",
      },
    ],
    created_at: "2026-08-20T05:00:00Z",
    ...over,
  };
}

// One act's proposals, all carrying the act's `bundle_id`: the API decides them
// in one call, so the deck reads them as ONE question with three items behind an
// expander rather than as three answers to something decided once.
export const BUNDLE = "018f3a1b-0000-7000-8000-0000000000b1";

export const bundle: Approval[] = [
  proposal("ap-facts", "Publish the acme.example company facts", {
    kind: "deepread",
    bundle_id: BUNDLE,
    proposed_change: { source_url: "https://acme.example" },
  }),
  proposal("ap-lead-1", "Lead from acme.example: Anna Weber", {
    kind: "site_lead",
    bundle_id: BUNDLE,
    proposed_change: { name: "Anna Weber", role: "Head of Operations" },
  }),
  proposal("ap-lead-2", "Lead from acme.example: Mira Osei", {
    kind: "site_lead",
    bundle_id: BUNDLE,
    proposed_change: { name: "Mira Osei", role: "Procurement" },
  }),
];

export const singles: Approval[] = [
  proposal("ap-1", "Send the follow-up to Anna Weber"),
  proposal("ap-2", "Move the PIM rollout to Proposal", {
    kind: "advance_deal",
    proposed_change: {
      current_stage: "Qualified",
      proposed_stage: "Proposal",
    },
  }),
  proposal("ap-3", "Promote Kilian Wenzel to a contact", {
    kind: "promote_lead",
    proposed_change: { name: "Kilian Wenzel", organization: "Nordwind" },
  }),
];

// A proposal nobody answered in time. A fixed instant in the past, so this frame
// documents the lapsed card for as long as the catalog exists: no Accept at all,
// because a control whose only possible answer is a refusal is worse than none.
export const lapsed = proposal("ap-lapsed", "Send the Q3 price list", {
  expires_at: "2026-07-01T09:00:00Z",
});

export function deal(id: string, name: string, over: Partial<Deal> = {}): Deal {
  return {
    id,
    name,
    amount_minor: 4_800_000,
    currency: "EUR",
    pipeline_id: "018f3a1b-0000-7000-8000-00000000p001",
    stage_id: "018f3a1b-0000-7000-8000-000000000s02",
    status: "open",
    stalled: false,
    source: "manual",
    captured_by: "human:018f3a1b-0000-7000-8000-000000000001",
    version: 1,
    created_at: "2026-05-01T08:00:00Z",
    updated_at: "2026-08-19T08:00:00Z",
    last_activity_at: "2026-08-19T08:00:00Z",
    ...over,
  };
}

export const deals: Deal[] = [
  deal("d-1", "Fleet retrofit", { organization_id: "org-nordwind" }),
  deal("d-2", "PIM rollout", {
    amount_minor: 2_650_000,
    organization_id: "org-acme",
  }),
  deal("d-3", "Depot lighting", { amount_minor: 890_000, currency: "USD" }),
  // The two that have gone quiet: open, stalled, and named — the rail resolves
  // the company through the same naming the pipeline board uses.
  deal("d-9", "Ostwind refit", {
    amount_minor: 1_200_000,
    organization_id: "org-nordwind",
    stalled: true,
    last_activity_at: "2026-06-02T08:00:00Z",
  }),
  deal("d-10", "Cold store retrofit", {
    amount_minor: 3_400_000,
    organization_id: "org-acme",
    stalled: true,
    last_activity_at: "2026-05-28T08:00:00Z",
  }),
];

export const briefItem = (
  id: string,
  dealId: string,
  rank: number,
  composite: number,
): MorningBrief["items"][number] => ({
  id,
  deal_id: dealId,
  rank,
  composite,
  feature_vector: {
    winnability: 0.4 + rank * 0.1,
    revenue: 1 - rank * 0.2,
    timing: 0.75,
    momentum: 0.9 - rank * 0.15,
    warmth: 0.47,
  },
  evidence_ids: ["ev-1", "ev-2"],
  state: "new",
  state_at: null,
});

export const ranked: MorningBrief = {
  id: "br-1",
  generated_at: "2026-08-21T05:30:00Z",
  as_of: "2026-08-21T05:00:00Z",
  candidate_count: 9,
  items: [
    briefItem("bi-1", "d-1", 1, 0.74),
    briefItem("bi-2", "d-2", 2, 0.61),
    briefItem("bi-3", "d-3", 3, 0.44),
  ],
};

/** A run that ranked nothing. Honest quiet, and no invented urgency. */
export const quietRun: MorningBrief = {
  ...ranked,
  candidate_count: 0,
  items: [],
};

export const digest: MorningDigest = {
  date: "2026-08-20",
  generated_at: "2026-08-21T03:00:00Z",
  capture: {
    messages_synced: 42,
    activities_created: 42,
    people_created: 5,
    organizations_created: 2,
  },
  review: {
    dedupe_open: 3,
    approvals_pending: 4,
    classify: { commitments: 4, meetings: 2, noise: 30 },
  },
  connectors: [
    { provider: "gmail", status: "connected" },
    { provider: "gcal", status: "connected" },
  ],
  projects: {
    phase_changes: [
      {
        project_id: "01a00000-0000-7000-8000-000000000001",
        name: "ERP replacement",
        from_phase: "pursuing",
        to_phase: "delivering",
        occurred_at: "2026-08-21T01:00:00Z",
      },
    ],
    new_commitments: [],
    gone_quiet: [
      {
        project_id: "01a00000-0000-7000-8000-000000000002",
        name: "Depot rollout",
        phase: "delivering",
        quiet_since: "2026-07-12T01:00:00Z",
        days_quiet: 40,
      },
    ],
  },
};

// The open pipeline, per currency. Two of them, because that is the case worth
// looking at: they get a line each rather than a sum, since adding native minor
// units across currencies produces a number that is not money.
export const pipelineRows = [
  {
    currency: "EUR",
    deals: 14,
    raw_minor: 9_900_000,
    weighted_minor: 3_100_000,
  },
  { currency: "USD", deals: 3, raw_minor: 2_400_000, weighted_minor: 900_000 },
];

export function report(rows: unknown[], excluded = 0): Response {
  return jsonResponse({
    report: "deals-by-stage",
    plan: {},
    columns: [],
    excluded_by_permission: excluded,
    rows,
  });
}

export const NOT_FOUND = { title: "Not Found", code: "no_digest_yet" };

// ── The readings strip ──────────────────────────────────────────────────────

export type Worklist = components["schemas"]["Worklist"];
type Readings = components["schemas"]["WorklistReadings"];

/** One meeting row, prepared or not, as the queue would carry it. */
export function meetingRow(id: string, prepared: boolean): WorklistItem {
  return {
    id,
    source: "meeting",
    level: 3,
    category: "meetings",
    title: "Weber GmbH · quarterly review",
    because: prepared ? [] : [{ kind: "meeting_unprepared" }],
    consequence: prepared ? "none" : "meeting_unprepared",
    actions: ["open"],
  };
}

/**
 * A lead owed a first answer, optionally naming when that answer is due.
 *
 * `due` undefined is the breached case: the moment has already passed, so the
 * row carries `response_overdue` and names no future deadline.
 */
export function leadRow(id: string, due?: string): WorklistItem {
  return {
    id,
    source: "lead_response",
    level: due === undefined ? 1 : 2,
    category: "leads",
    title: "Weber GmbH · inbound enquiry",
    because:
      due === undefined
        ? // A breached lead the way the lane really builds one: the overdue
          // reason plus how long it has been waiting. The second reason CARRIES
          // A VALUE, which is what makes this fixture able to tell a slot that
          // filters on the reason kind from one that merely takes the first
          // value it meets.
          [
            { kind: "response_overdue" },
            { kind: "waiting_days", value: { kind: "days", days: 3 } },
          ]
        : [{ kind: "response_due_soon", value: { kind: "date", date: due } }],
    consequence: "buyer_waits",
    actions: ["open"],
  };
}

/** A leads count the page carries whole — considered, shown and read to the end. */
export function wholeLeads(n: number): WorklistCount {
  return {
    category: "leads",
    considered: n,
    shown: n,
    more_available: false,
  };
}

/** A leads count whose read stopped at its bound: more exist than the page shows. */
export function boundedLeads(considered: number, shown: number): WorklistCount {
  return {
    category: "leads",
    considered,
    shown,
    more_available: true,
  };
}

type WorklistItem = components["schemas"]["WorklistItem"];

/**
 * The overnight run's suggestion as the WORKLIST carries it.
 *
 * Same id as the `MorningBrief` item it came from — `briefItem` in
 * `attention/render.go` sends the entry's own id — which is what lets the Focus
 * section leave out whatever Do next already drew.
 */
export function overnightRow(id: string, dealId: string): WorklistItem {
  return {
    id,
    source: "brief_item",
    level: 3,
    category: "deals_at_risk",
    because: [],
    consequence: "deal_drifts",
    actions: ["act", "set_aside", "dismiss"],
    subject: { type: "deal", id: dealId },
  };
}

/** One customer waiting on an answer — the row Do next leads with. */
export function waitingRow(): WorklistItem {
  return {
    id: "w1",
    source: "customer_waiting",
    level: 1,
    category: "customer_waiting",
    title: "Aster Handel",
    because: [],
    consequence: "buyer_waits",
    actions: ["open"],
  };
}

type WorklistCount = components["schemas"]["WorklistCount"];

/**
 * A morning as the worklist answers it, for the readings strip.
 *
 * The meetings COUNT comes from `counts` — every meeting read and ranked, before
 * the fold and the page cut — while readiness can only be counted off the rows
 * the page carries. The default makes them agree, which is the ordinary day; a
 * case that needs them to DISAGREE overrides `counts`, and the strip must then
 * refuse to state a readiness figure rather than dividing one population by the
 * other.
 */
export function readingsDay(
  readings: Partial<Readings> = {},
  queue: WorklistItem[] = [meetingRow("m1", false), meetingRow("m2", true)],
  counts: WorklistCount[] = [wholeMeetings(queue.length)],
): Worklist {
  return {
    as_of: "2026-08-31T06:42:00Z",
    scope: "mine",
    scope_options: ["mine"],
    queue,
    summary: { urgent: 0, due: 0, lower_priority: 0, total: queue.length },
    sources_unavailable: [],
    reach: [],
    counts,
    readings: {
      revenue_at_risk_minor: null,
      buyer_replies: 3,
      prospecting: 2,
      review: 8,
      more_available: false,
      ...readings,
    },
  };
}

/** A meetings count the page carries whole — considered, shown and read to the end. */
export function wholeMeetings(n: number): WorklistCount {
  return {
    category: "meetings",
    considered: n,
    shown: n,
    more_available: false,
  };
}

/** A meetings count where more meetings were ranked than the page carries. */
export function boundedMeetings(
  considered: number,
  shown: number,
): WorklistCount {
  return { category: "meetings", considered, shown, more_available: true };
}
