// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { HomeScreen } from "./home";
import type { Deal, MorningBrief, MorningDigest } from "./home.queries";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// Home — the morning handover, in the states a reader actually arrives at.
//
// The page has two moods and the order between them is the whole design: while
// decisions are waiting they LEAD (they are the only thing here with a
// deadline), and once the deck is clear the ranked queue leads. Both are frames
// below, because a catalog that only ever showed one of them would document
// half a page.
//
// Read every frame in BOTH themes with the toolbar's Theme control (it flips
// `data-theme` exactly the way the shell does). Nothing here is theme-aware in
// its own right, and that is precisely why it needs looking at: every colour on
// the deck's urgency edge, the staging tray, the readings strip and the rail's
// panels is a `color-mix()` of a canonical token, so a surface can be correct in
// light and wrong in dark.
//
// EVERY INSTANT IS FIXED. A fixture built with `new Date()` documents whatever
// day the catalog was opened on, and the two things on this page that read a
// clock — the greeting band and a proposal's expiry — would then say something
// different every time somebody looked. The one exception is deliberate and
// unavoidable: the greeting reads the real hour, because Home passes it its own
// clock. Expiries are therefore either ABSENT (calm, and stable forever) or a
// fixed instant in the past (the lapsed frame, which stays lapsed).

type Approval = components["schemas"]["Approval"];

// ── Fixtures ────────────────────────────────────────────────────────────────

/** One staged proposal, named by the sentence its card leads with. */
function proposal(
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
const BUNDLE = "018f3a1b-0000-7000-8000-0000000000b1";

const bundle: Approval[] = [
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

const singles: Approval[] = [
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
const lapsed = proposal("ap-lapsed", "Send the Q3 price list", {
  expires_at: "2026-07-01T09:00:00Z",
});

function deal(id: string, name: string, over: Partial<Deal> = {}): Deal {
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

const deals: Deal[] = [
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

const briefItem = (
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

const ranked: MorningBrief = {
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
const quietRun: MorningBrief = { ...ranked, candidate_count: 0, items: [] };

const digest: MorningDigest = {
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
const pipelineRows = [
  {
    currency: "EUR",
    deals: 14,
    raw_minor: 9_900_000,
    weighted_minor: 3_100_000,
  },
  { currency: "USD", deals: 3, raw_minor: 2_400_000, weighted_minor: 900_000 },
];

function report(rows: unknown[], excluded = 0): Response {
  return jsonResponse({
    report: "deals-by-stage",
    plan: {},
    columns: [],
    excluded_by_permission: excluded,
    rows,
  });
}

const NOT_FOUND = { title: "Not Found", code: "no_digest_yet" };

// ── The harness ─────────────────────────────────────────────────────────────

type Frame = {
  /** The pending queue. Every frame states it, because "none waiting" is the
   *  state that flips the page's order and is never a default worth guessing. */
  approvals: Approval[];
  /** The ranked run, or null for the honest 404 (no run has been made yet). */
  brief: MorningBrief | null;
  /** The nightly digest, or null for the 404 an installation answers before its
   *  first run. */
  digest?: MorningDigest | null;
  /** What the pipeline report answers. A refusal is a state of its own. */
  pipeline?: () => Response;
  /** Extra routes a frame's own play() needs. */
  extra?: RouteMap;
};

/**
 * One Home, with every read it fans out to answered.
 *
 * Five independent reads and no combined "my day" endpoint, so each of them is
 * routed on its own here — which is the point rather than bookkeeping: a frame
 * can refuse ONE of them and show that the other four still render.
 */
function home({
  approvals,
  brief,
  digest: overnight = digest,
  pipeline = () => report(pipelineRows),
  extra = {},
}: Frame) {
  return () => {
    // Mutable per render so a play() that commits a verdict sees the queue the
    // commit left behind, rather than the one it started with.
    const decided = new Set<string>();
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /approvals": () =>
        jsonResponse({
          data: approvals.filter((approval) => !decided.has(approval.id)),
          page: { next_cursor: null, has_more: false },
        }),
      "POST /approvals/ap-1/approve": () => {
        decided.add("ap-1");
        return jsonResponse({ ...singles[0], status: "approved" });
      },
      "GET /brief": () =>
        brief ? jsonResponse(brief) : jsonResponse(NOT_FOUND, 404),
      "GET /digest": () =>
        overnight ? jsonResponse(overnight) : jsonResponse(NOT_FOUND, 404),
      "GET /deals": () =>
        jsonResponse({ data: deals, page: { next_cursor: null } }),
      "GET /organizations/org-nordwind": () =>
        jsonResponse({ id: "org-nordwind", display_name: "Nordwind Logistik" }),
      "GET /organizations/org-acme": () =>
        jsonResponse({ id: "org-acme", display_name: "Acme Fördertechnik" }),
      "GET /projects/01a00000-0000-7000-8000-000000000001": () =>
        jsonResponse({
          id: "01a00000-0000-7000-8000-000000000001",
          name: "ERP replacement",
        }),
      "GET /projects/01a00000-0000-7000-8000-000000000002": () =>
        jsonResponse({
          id: "01a00000-0000-7000-8000-000000000002",
          name: "Depot rollout",
        }),
      "POST /reports/deals-by-stage": () => pipeline(),
      ...extra,
    });
    return (
      <StoryProviders>
        <HomeScreen />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof HomeScreen> = {
  title: "Shell/Home",
  component: HomeScreen,
};
export default meta;
type Story = StoryObj<typeof HomeScreen>;

// ── The deck ────────────────────────────────────────────────────────────────

// The morning it was designed for: four decisions waiting (three proposals and
// one act's bundle), a ranked queue under them, and the context rail beside.
// Decisions LEAD, because they are the only thing here with a deadline.
export const MorningDeck: Story = {
  render: home({ approvals: [...singles, ...bundle], brief: ranked }),
};

// The last card. "0 more behind" is drawn rather than hidden: a reader deciding
// one at a time is owed the size of what is left, including when it is nothing.
export const LastCard: Story = {
  render: home({ approvals: [singles[0]], brief: ranked }),
};

// The tray, which is the undo the backend does not have: a recorded decision
// cannot be reversed, so the verdict sits here — locally, nothing sent — until
// somebody presses commit.
export const StagedTray: Story = {
  render: home({ approvals: [...singles], brief: ranked }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Accept" }),
    );
    await canvas.findByText("1 decision staged");
  },
};

// The earned moment: everything the reader answered has gone, and the deck says
// how many and when. Reached by accepting one and deferring the other — "later"
// keeps its card pending, which is what leaves the deck with nothing waiting
// while the queue still holds something.
export const DeckCleared: Story = {
  render: home({ approvals: [singles[0], singles[1]], brief: ranked }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Accept" }),
    );
    await userEvent.click(await canvas.findByRole("button", { name: "Later" }));
    await userEvent.click(
      await canvas.findByRole("button", { name: "Send staged decisions" }),
    );
    await canvas.findByText("Deck clear");
  },
};

// Nothing is waiting, so the ORDER FLIPS: the ranked queue leads and the deck
// stands under it saying so. The question has stopped being "what needs me" and
// become "what do I do first".
export const RankedQueueLeads: Story = {
  render: home({ approvals: [], brief: ranked }),
};

// A proposal that ran out of time. The card keeps its place and its content —
// the reader still needs to know what was proposed — but the Accept control is
// gone rather than drawn to be refused.
export const ExpiredCard: Story = {
  render: home({ approvals: [lapsed], brief: ranked }),
};

// ── The ranked queue ────────────────────────────────────────────────────────

// The first morning: no run has ever been made, so the page offers to make one
// instead of drawing an empty queue that looks like a failure.
export const NoBriefYet: Story = {
  render: home({ approvals: [], brief: null }),
};

// A run that ranked nothing. The honest quiet, with no invented urgency —
// distinct from the frame above, which has no run at all.
export const QuietRun: Story = {
  render: home({ approvals: [], brief: quietRun }),
};

// ── The rail ────────────────────────────────────────────────────────────────

// Before the first nightly run there is no digest, so the Overnight panel is
// absent rather than a row of zeros: a fabricated count is worse than a missing
// one, because a reader cannot tell it apart from a real one.
export const DigestAbsent: Story = {
  render: home({ approvals: [...singles], brief: ranked, digest: null }),
};

// The one place connector health reaches a reader without visiting Settings. A
// degraded source is news — said in Settings' own vocabulary, with the way to
// fix it — while a healthy one stays silent, as it does in every other frame
// here: a permanent green row is noise.
export const ConnectorUnhealthy: Story = {
  render: home({
    approvals: [...singles],
    brief: ranked,
    digest: {
      ...digest,
      connectors: [
        {
          provider: "gmail",
          status: "reauth_required",
          last_sync_error_class: "auth",
        },
      ],
    },
  }),
};

// One read refused while the other four are healthy. That is the whole reason
// this page fans out to five independent reads: the pipeline says the figure
// could not be loaded — a refusal, not an absence — and the deck, the queue, the
// digest and the quiet list are untouched beside it.
export const OnePanelRefused: Story = {
  render: home({
    approvals: [...singles],
    brief: ranked,
    pipeline: () =>
      jsonResponse({ title: "Forbidden", code: "forbidden" }, 403),
  }),
};

// The other half of the same honesty: the figures ARRIVED, and a field mask kept
// rows out of them. Saying so is the difference between a partial answer and a
// wrong one.
export const PipelinePartial: Story = {
  render: home({
    approvals: [],
    brief: ranked,
    pipeline: () => report(pipelineRows, 4),
  }),
};
