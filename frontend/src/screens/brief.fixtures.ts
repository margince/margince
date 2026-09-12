// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { Deal, MorningBrief, MorningDigest } from "./brief.queries";
import { jsonResponse } from "./story-utils";

// The fixtures every Brief story is built from — the whole page in
// `brief.stories.tsx`, its parts one at a time in `brief.parts.stories.tsx`. They
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
    proposed_change: { name: "Kilian Wenzel", company: "Nordwind" },
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
  deal("d-1", "Fleet retrofit", { company_id: "company-nordwind" }),
  deal("d-2", "PIM rollout", {
    amount_minor: 2_650_000,
    company_id: "company-acme",
  }),
  deal("d-3", "Depot lighting", { amount_minor: 890_000, currency: "USD" }),
  // The two that have gone quiet: open, stalled, and named — the rail resolves
  // the company through the same naming the pipeline board uses.
  deal("d-9", "Ostwind refit", {
    amount_minor: 1_200_000,
    company_id: "company-nordwind",
    stalled: true,
    last_activity_at: "2026-06-02T08:00:00Z",
  }),
  deal("d-10", "Cold store retrofit", {
    amount_minor: 3_400_000,
    company_id: "company-acme",
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
  factors_omitted: [],
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
    contacts_created: 5,
    companies_created: 2,
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

export type WeeklyReview = components["schemas"]["WeeklyReview"];

// The week just gone, in ONE payload.
//
// Two story files draw this review — the whole morning and the panel on its own
// — and they had a fixture each, which is two answers to "what does an ordinary
// week look like". A part that drifts from the page it is part of is exactly the
// drift these frames exist to catch, so it cannot be two literals.
//
// EVERY INSTANT IS FIXED. A review built from `new Date()` documents whichever
// day the catalog was opened on, and both dates the panel prints — the week it
// names and the day each deal line closed — would then say something different
// every time somebody looked.

export const WEEK_START = "2026-06-29";
export const PRIOR_WEEK_START = "2026-06-22";

const weeklyCounts: WeeklyReview["counts"] = {
  tasks_due: 6,
  tasks_done: 4,
  tasks_carried_over: 2,
  deals_moved: 3,
  deals_won: 3,
  deals_lost: 1,
  proposals_accepted: 8,
  proposals_rejected: 2,
  brief_items_acted: 7,
  brief_items_dismissed: 4,
  commitments_due: 4,
  commitments_kept: 3,
  leads_routed: 9,
  leads_answered_in_target: 7,
  leads_breached: 2,
  meetings_held: 5,
  meetings_with_next_step: 3,
};

/**
 * A rep's first review: measured, and with nothing behind it to measure
 * against.
 *
 * Its own fixture rather than the ordinary week minus a field, because "no
 * prior" is what every other frame here is built on top of.
 */
export const firstWeek: WeeklyReview = {
  id: "01a04000-0000-7000-8000-00000000000a",
  local_week_start: WEEK_START,
  generated_at: "2026-07-06T06:00:00Z",
  as_of: "2026-07-06T06:00:00Z",
  counts: weeklyCounts,
  deals: [],
};

/**
 * What the week did to the pipeline, converted and frozen.
 *
 * Its own value rather than folded into the week below, because ABSENT is a
 * real state with its own frame: an open deal freezes no rate, so one
 * unconvertible deal makes the whole figure unanswerable and the panel prints
 * the count instead of a sum. A week that always carries money cannot show
 * that the two are different sentences.
 */
export const weeklyPipeline: NonNullable<WeeklyReview["pipeline"]> = {
  created_minor: 214_000_00,
  won_minor: 96_500_00,
  lost_minor: 31_000_00,
  currency: "EUR",
};

/** The ordinary Monday: a sentence, a week before it, and three closed lines. */
export const narratedWeek: WeeklyReview = {
  ...firstWeek,
  pipeline: weeklyPipeline,
  narrative: "Weber signed on Thursday; two promises slipped into this week.",
  narrated_at: "2026-07-06T06:01:00Z",
  prior: {
    local_week_start: PRIOR_WEEK_START,
    // Deliberately uneven against the week above: one figure up, one down and
    // one exactly level, so the delta line is readable as arithmetic a reader
    // can check rather than as five copies of the same string. The level one is
    // the case worth the frame — it prints "±0", never "+0".
    counts: { ...weeklyCounts, deals_won: 1, meetings_held: 7 },
  },
  // All three outcome words, because each is a separate lookup and a stage
  // label rides along with only one of them. The labels are what the deals were
  // CALLED that week: frozen, so they render even though nothing here serves
  // the deals they name.
  deals: [
    {
      deal_id: "01a04000-0000-7000-8000-00000000000b",
      label: "Weber Rahmenvertrag",
      outcome: "won",
      occurred_at: "2026-07-02T14:00:00Z",
    },
    {
      deal_id: "01a04000-0000-7000-8000-00000000000c",
      label: "Aster Handel — Kassensystem",
      outcome: "lost",
      occurred_at: "2026-07-03T09:30:00Z",
    },
    {
      deal_id: "01a04000-0000-7000-8000-00000000000d",
      label: "Nordwind Logistik — Depot rollout",
      outcome: "moved",
      to_stage_label: "Proposal sent",
      occurred_at: "2026-07-01T16:15:00Z",
    },
  ],
};

/**
 * Where the week was landing, frozen at all three horizons.
 *
 * Three entries rather than one, because the horizon dial only exists where
 * there is something to switch BETWEEN — a single-entry outlook draws a control
 * whose one option is the one already showing. The quarter carries both
 * landings and its movement bars, so the bridge under the strip has an opening
 * to have moved from; the week and the month carry the strip alone, which is
 * the other honest shape and the one that makes the bridge say "no opening".
 */
export const weeklyOutlook: NonNullable<WeeklyReview["outlook"]> = [
  {
    period_kind: "week",
    period_start: "2026-06-29",
    period_end: "2026-07-05",
    base_currency: "EUR",
    won_minor: 96_500_00,
    commit_minor: 42_000_00,
    best_case_minor: 78_000_00,
    weighted_minor: 55_400_00,
    movement: [],
  },
  {
    period_kind: "month",
    period_start: "2026-07-01",
    period_end: "2026-07-31",
    base_currency: "EUR",
    won_minor: 96_500_00,
    commit_minor: 214_000_00,
    best_case_minor: 356_000_00,
    weighted_minor: 248_200_00,
    movement: [],
  },
  {
    period_kind: "quarter",
    period_start: "2026-07-01",
    period_end: "2026-09-30",
    base_currency: "EUR",
    won_minor: 96_500_00,
    commit_minor: 512_000_00,
    best_case_minor: 940_000_00,
    weighted_minor: 604_500_00,
    opening_landing_minor: 572_000_00,
    closing_landing_minor: 604_500_00,
    forward_measure: "commit_evidence",
    // The bars sum to the move between the two landings, because a bridge that
    // does not reconcile draws its own warning.
    movement: [
      {
        bar: "created",
        delta_minor: 84_000_00,
        drivers: [
          {
            deal_id: "01a04000-0000-7000-8000-00000000000e",
            deal_label: "Hansa Werft — Werkstattsteuerung",
            delta_minor: 84_000_00,
          },
        ],
      },
      { bar: "advanced", delta_minor: 26_500_00 },
      { bar: "slipped", delta_minor: -47_000_00 },
      { bar: "lost", delta_minor: -31_000_00 },
    ],
  },
];

/**
 * How well the week went, both blocks answered.
 *
 * A block the server did not send is a REP WHO CARRIED NONE of that work, so
 * every story that means to show one block absent drops it from this value
 * rather than zeroing its fields.
 */
export const weeklyScorecard: NonNullable<WeeklyReview["scorecard"]> = {
  lead: {
    advanced: 4,
    disqualified: 1,
    promoted: 2,
    answered_in_target: 7,
    breached: 2,
    meetings_booked: 6,
    meetings_held: 5,
    meetings_no_show: 1,
    meetings_partial_history: 0,
  },
  deal: {
    advances: 7,
    regressions: 2,
    median_days_in_stage: 11,
    with_next_step: 3,
    open: 5,
    multi_threaded: 2,
    close_date_sound: 4,
    forecast_up: 3,
    forecast_down: 1,
  },
};

/**
 * What the week taught, with what each lesson rests on.
 *
 * One of each shape the panel labels, and every one carries a citation: a
 * learning stored without one is refused, so a fixture without one would
 * document a row the server cannot produce.
 */
export const weeklyLearnings: NonNullable<WeeklyReview["learnings"]> = {
  state: "synthesized",
  items: [
    {
      kind: "worked",
      text: "Reaching the sponsor before the technical review won Weber.",
      citations: [
        {
          subject_type: "deal",
          subject_id: "01a04000-0000-7000-8000-00000000000b",
          label: "Weber Rahmenvertrag",
        },
      ],
    },
    {
      kind: "did_not_work",
      text: "Aster went quiet after the price list went out with no call behind it.",
      citations: [
        {
          subject_type: "deal",
          subject_id: "01a04000-0000-7000-8000-00000000000c",
          label: "Aster Handel — Kassensystem",
        },
      ],
    },
    {
      kind: "pattern",
      text: "Every deal that closed this quarter had a second contact by week two.",
      citations: [
        {
          subject_type: "deal",
          subject_id: "01a04000-0000-7000-8000-00000000000d",
          label: "Nordwind Logistik — Depot rollout",
        },
      ],
    },
    {
      kind: "experiment",
      text: "Book the follow-up in the meeting rather than after it.",
      citations: [
        {
          subject_type: "commitment",
          subject_id: "01a04000-0000-7000-8000-00000000001a",
          label: "Send the Weber quote",
        },
      ],
    },
  ],
};

/** The Monday with every lane answered: figures, landing, scorecard, lessons. */
export const wholeWeek: WeeklyReview = {
  ...narratedWeek,
  outlook: weeklyOutlook,
  scorecard: weeklyScorecard,
  learnings: weeklyLearnings,
};

export type TeamWeeklyReview = components["schemas"]["TeamWeeklyReview"];
export type Team = components["schemas"]["Team"];

/** The one team a lead reads, for the picker above the team's week. */
export const team: Team = { id: "t-nord", name: "Nord" };

/**
 * A team's week as it was measured when the week closed.
 *
 * Three reps with three different verdicts, because the agenda is an ORDER over
 * them and a fixture where everybody had the same week documents no ordering at
 * all. `agenda` is not optional on the wire, so it is spelled here rather than
 * left to a default the server never sends.
 */
export const teamWeek: TeamWeeklyReview = {
  id: "01a04000-0000-7000-8000-0000000000f1",
  team_id: team.id,
  team_name: team.name,
  local_week_start: WEEK_START,
  generated_at: "2026-07-06T06:00:00Z",
  as_of: "2026-07-06T06:00:00Z",
  reps_unread: 0,
  counts: {
    reps_counted: 3,
    deals_won: 4,
    deals_lost: 1,
    deals_moved: 9,
    leads_routed: 14,
    leads_answered_in_target: 13,
    leads_breached: 1,
    meetings_held: 12,
    meetings_with_next_step: 6,
    commitments_due: 9,
    commitments_kept: 7,
  },
  reps: [
    {
      user_id: "u-lena",
      display_name: "Lena Fischer",
      deals_won: 2,
      leads_breached: 0,
      meetings_held: 5,
      commitments_due: 3,
      commitments_kept: 3,
      help_requested: 0,
      focus_kind: "strong_week",
      focus_label: "Fastest first response on the team",
    },
    {
      user_id: "u-tobias",
      display_name: "Tobias Kern",
      deals_won: 1,
      leads_breached: 1,
      meetings_held: 4,
      commitments_due: 4,
      commitments_kept: 2,
      help_requested: 1,
      focus_kind: "help_requested",
      focus_label: "Asked for help on the Hansa renewal",
    },
    {
      user_id: "u-mira",
      display_name: "Mira Sandoval",
      deals_won: 1,
      leads_breached: 0,
      meetings_held: 3,
      commitments_due: 2,
      commitments_kept: 2,
      help_requested: 0,
      focus_kind: "meetings_without_next_step",
      focus_label: "Three meetings closed without a next step",
    },
  ],
  // Who is talked about first: the contact who asked for help, then the one
  // whose meetings ended open, then the week that went well.
  agenda: ["u-tobias", "u-mira", "u-lena"],
  outlook: weeklyOutlook,
};

export type Worklist = components["schemas"]["Worklist"];
type Readings = components["schemas"]["WorklistReadings"];

/**
 * One meeting row, prepared or not, as the queue would carry it.
 *
 * `due_at` is the start, and it is not optional in the fixture because it is
 * not optional on the wire: the meeting lane sets it on every row it produces.
 * A fixture that left it off was the reason a panel which could never draw a
 * time had a green test suite.
 */
export function meetingRow(
  id: string,
  prepared: boolean,
  startsAt = "2026-09-03T09:30:00Z",
): WorklistItem {
  return {
    id,
    source: "meeting",
    level: 3,
    category: "meetings",
    title: "Weber GmbH · quarterly review",
    due_at: startsAt,
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
  // The summary is overridable because the strip's first reading comes from it
  // rather than from `readings`: a fixture that pinned it at zero could only
  // ever exercise the empty case of the card a rep reads first.
  summary: Partial<Worklist["summary"]> = {},
): Worklist {
  return {
    as_of: "2026-08-31T06:42:00Z",
    scope: "mine",
    scope_options: ["mine"],
    queue,
    summary: {
      urgent: 0,
      due: 0,
      lower_priority: 0,
      total: queue.length,
      ...summary,
    },
    sources_unavailable: [],
    reach: [],
    counts,
    readings: {
      changed_since_brief: 0,
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

/**
 * A decision on the queue, and whether it actually holds customer work up.
 *
 * The split is the ranker's own: a decision about a SEND is blocking, and
 * contact hygiene — a duplicate pair, a captured counterparty — is not
 * (classifydecision.go). A fixture that made every decision blocking could not
 * tell the strip's two basis lines apart.
 */
export function decisionRow(id: string, blocking: boolean): WorklistItem {
  return {
    id,
    source: "approval",
    // `kind` is what the server actually decides on: blocksCustomerWork reads
    // it and treats an absent one as hygiene, so a fixture without it could
    // not be the blocking row it claims to be.
    kind: blocking ? "send_email" : "capture_counterparty",
    level: blocking ? 5 : 6,
    category: "decisions",
    title: blocking ? "Send the renewal quote" : "Add someone from your mail",
    because: [{ kind: blocking ? "blocks_customer_work" : "routine" }],
    consequence: blocking ? "work_blocked" : "data_drifts",
    actions: ["decide"],
  };
}

/** A decisions count read to the end. */
export function wholeDecisions(n: number): WorklistCount {
  return {
    category: "decisions",
    considered: n,
    shown: n,
    more_available: false,
  };
}

/** A decisions count whose lane stopped at its bound. */
export function boundedDecisions(
  considered: number,
  shown: number,
): WorklistCount {
  return { category: "decisions", considered, shown, more_available: true };
}
