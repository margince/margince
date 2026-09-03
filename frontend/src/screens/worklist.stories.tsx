import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { jsonResponse, StoryProviders } from "./story-utils";
import { WorklistScreen } from "./worklist";

// The ranked queue, in the three readings that matter: a full day, a clear one,
// and one the page could not read the whole of.

type Worklist = components["schemas"]["Worklist"];
type TeamBoard = components["schemas"]["TeamBoard"];

// A team with somebody carrying more than the rest, which is what a lead opens
// the board to find.
const aLoadedTeam: TeamBoard = {
  as_of: "2026-08-31T09:00:00Z",
  members: [
    {
      user_id: "00000000-0000-4000-8000-000000000001",
      display_name: "Lena Fischer",
      counts: { waiting: 14, at_risk: 3, overdue: 6 },
    },
    {
      user_id: "00000000-0000-4000-8000-000000000002",
      display_name: "Marc Weber",
      counts: { waiting: 2, at_risk: 0, overdue: 0 },
    },
    {
      user_id: "00000000-0000-4000-8000-000000000003",
      display_name: "Sofia Ruiz",
      counts: { waiting: 0, at_risk: 1, overdue: 0 },
    },
  ],
  unassigned: { waiting: 3, at_risk: 0, overdue: 1 },
  truncated: false,
};

// The board is fetched by the screen whenever the reader's scope reaches
// `team`, so the day stub has to answer it: matching /worklist alone would hand
// the board a Worklist and the table would draw from a shape it does not have.
function stubDay(day: Worklist, board: TeamBoard = aLoadedTeam) {
  globalThis.fetch = (async (input: RequestInfo | URL): Promise<Response> => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.includes("/worklist/team")) {
      return jsonResponse(board);
    }
    if (url.includes("/worklist")) {
      return jsonResponse(day);
    }
    return jsonResponse({ data: [] });
  }) as typeof fetch;
}

const meta: Meta<typeof WorklistScreen> = {
  title: "Records/Worklist/Screen",
  component: WorklistScreen,
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj<typeof WorklistScreen>;

// A seller's ordinary morning: a customer waiting at the top, a material deal
// under it, and the routine decisions last — which is the whole point of the
// ordering. On the surface this replaces, the decisions came first.
export const AFullDay: Story = {
  render: () => {
    stubDay({
      as_of: "2026-08-31T09:00:00Z",
      scope: "mine",
      scope_options: ["mine", "unassigned", "team", "all"],
      summary: { urgent: 1, due: 2, lower_priority: 3, total: 5 },
      sources_unavailable: [],
      reach: [],
      // A full day: money drifting, buyers waiting, a decision pile.
      readings: {
        revenue_at_risk_minor: 384_500_00,
        revenue_currency: "EUR",
        buyer_replies: 14,
        prospecting: 3,
        review: 27,
        more_available: false,
      },
      // More work than the page carries, which is the ordinary case and the one
      // these figures exist for: four decisions sit behind the single row drawn.
      counts: [
        {
          category: "customer_waiting",
          considered: 1,
          shown: 1,
          more_available: false,
        },
        {
          category: "deals_at_risk",
          considered: 1,
          shown: 1,
          more_available: false,
        },
        { category: "tasks", considered: 1, shown: 1, more_available: false },
        {
          category: "decisions",
          considered: 5,
          shown: 1,
          more_available: false,
        },
      ],
      queue: [
        {
          id: "waiting-1",
          source: "customer_waiting",
          category: "customer_waiting",
          level: 1,
          consequence: "buyer_waits",
          title: "Re: pricing for the retrofit",
          because: [
            { kind: "buyer_wrote_last" },
            { kind: "waiting_days", value: { kind: "days", days: 83 } },
          ],
          above_next: {
            comparator: "level",
            mine: { kind: "level", level: 1 },
            theirs: { kind: "level", level: 3 },
          },
          actions: ["open"],
        },
        {
          id: "deal-1",
          source: "deal_at_risk",
          category: "deals_at_risk",
          level: 3,
          consequence: "deal_slips_past_close",
          title: "Acme Expansion",
          overdue: true,
          because: [
            {
              kind: "material",
              value: { kind: "money", minor: 16010000, currency: "EUR" },
            },
            { kind: "quiet_days", value: { kind: "days", days: 83 } },
          ],
          above_next: {
            comparator: "deadline",
            mine: { kind: "date", date: "2026-08-26T00:00:00Z" },
            theirs: { kind: "date", date: "2026-09-28T00:00:00Z" },
          },
          actions: ["open"],
        },
        {
          id: "task-1",
          source: "task",
          category: "tasks",
          level: 4,
          consequence: "task_slips",
          title: "Confirm the workshop date",
          because: [{ kind: "due_today" }],
          actions: [],
        },
        {
          id: "approval-1",
          source: "approval",
          category: "decisions",
          level: 6,
          consequence: "data_drifts",
          title: "Add someone from your mail",
          because: [{ kind: "routine" }],
          actions: ["decide"],
        },
      ],
    });
    return (
      <StoryProviders>
        <WorklistScreen />
      </StoryProviders>
    );
  },
};

// The state the whole surface is built to reach. One line, and no card drawn
// to report a zero.
export const NothingWaiting: Story = {
  render: () => {
    stubDay({
      as_of: "2026-08-31T09:00:00Z",
      scope: "mine",
      scope_options: ["mine"],
      summary: { urgent: 0, due: 0, lower_priority: 0, total: 0 },
      sources_unavailable: [],
      reach: [],
      // Nothing waiting: every figure honestly zero, and the money priced.
      readings: {
        revenue_at_risk_minor: 0,
        revenue_currency: "EUR",
        buyer_replies: 0,
        prospecting: 0,
        review: 0,
        more_available: false,
      },
      // A genuinely clear day: nothing read, nothing counted, nothing hidden.
      counts: [],
      queue: [],
    });
    return (
      <StoryProviders>
        <WorklistScreen />
      </StoryProviders>
    );
  },
};

// A day the page could not read the whole of. The warning is the surface
// speaking about itself, and the clear line never prints beside it.
export const PartlyUnread: Story = {
  render: () => {
    stubDay({
      as_of: "2026-08-31T09:00:00Z",
      scope: "mine",
      scope_options: ["mine"],
      summary: { urgent: 0, due: 0, lower_priority: 0, total: 0 },
      sources_unavailable: [{ source: "capture_health", reason: "failed" }],
      queue: [],
      reach: [],
      // Partly unread: the figures are floors, so the strip says so.
      readings: {
        revenue_at_risk_minor: 42_000_00,
        revenue_currency: "EUR",
        buyer_replies: 2,
        prospecting: 1,
        review: 5,
        more_available: true,
      },
      // Nothing counted, because the source that would have filled the day is
      // the one that could not be read. The warning says so; the counts do not
      // pretend otherwise.
      counts: [],
    });
    return (
      <StoryProviders>
        <WorklistScreen />
      </StoryProviders>
    );
  },
};

// A lead's own morning, with the team above it.
//
// The board is the second question, which is why it is behind a disclosure: the
// lead's own day is still what they came for. Lena is carrying fourteen waiting
// customers to Marc's two, which is the reading the whole surface exists for —
// and the unassigned row under them is work nobody is looking at at all.
export const ALeadsDay: Story = {
  render: () => {
    stubDay({
      as_of: "2026-08-31T09:00:00Z",
      scope: "mine",
      scope_options: ["mine", "unassigned", "team", "all"],
      summary: { urgent: 0, due: 1, lower_priority: 0, total: 1 },
      sources_unavailable: [],
      reach: [],
      // A leads day: prospecting carries it, and nothing at risk could be priced.
      readings: {
        revenue_at_risk_minor: null,
        buyer_replies: 1,
        prospecting: 9,
        review: 0,
        more_available: false,
      },
      counts: [
        { category: "tasks", considered: 1, shown: 1, more_available: false },
      ],
      queue: [
        {
          id: "task-1",
          source: "task",
          category: "tasks",
          level: 4,
          consequence: "task_slips",
          title: "Confirm the workshop date",
          because: [{ kind: "due_today" }],
          actions: [],
        },
      ],
    });
    return (
      <StoryProviders>
        <WorklistScreen />
      </StoryProviders>
    );
  },
};

// The board reporting FLOORS rather than totals.
//
// A count read to its source's bound cannot say how much sits past it, so the
// line under the table says the figures are a floor. Without it a lead told
// "50" over a number that is really 50-or-more would not go looking, which is
// the one direction this surface must not get wrong.
export const ATeamBiggerThanTheBoardCanCount: Story = {
  render: () => {
    stubDay(
      {
        as_of: "2026-08-31T09:00:00Z",
        scope: "mine",
        scope_options: ["mine", "unassigned", "team", "all"],
        summary: { urgent: 0, due: 0, lower_priority: 0, total: 0 },
        sources_unavailable: [],
        reach: [],
        // A team bigger than the board can count: every figure a floor.
        readings: {
          revenue_at_risk_minor: 1_250_000_00,
          revenue_currency: "EUR",
          buyer_replies: 61,
          prospecting: 24,
          review: 140,
          more_available: true,
        },
        counts: [],
        queue: [],
      },
      { ...aLoadedTeam, truncated: true },
    );
    return (
      <StoryProviders>
        <WorklistScreen />
      </StoryProviders>
    );
  },
};

// A day of things that went wrong, each saying WHAT went wrong.
//
// The story worth looking at for this change: six system rows whose supporting
// line was on the wire all along and drawn for none of them. A rep reading
// "A mailbox stopped syncing" could not tell which mailbox, and "Automation
// failed" named neither the rule nor the reason.
//
// The group row is the other half. Its `cause` is the identity twelve failures
// were folded on and reads like one; the sentence is written from `label`, so
// the row says which rule broke rather than "automation_run:01a0… failed 12
// times".
export const WhatWentWrong: Story = {
  render: () => {
    stubDay({
      as_of: "2026-08-31T09:00:00Z",
      scope: "mine",
      scope_options: ["mine"],
      summary: { urgent: 0, due: 2, lower_priority: 4, total: 6 },
      sources_unavailable: [],
      reach: [],
      readings: {
        revenue_at_risk_minor: 0,
        revenue_currency: "EUR",
        buyer_replies: 0,
        prospecting: 0,
        review: 0,
        more_available: false,
      },
      counts: [],
      queue: [
        {
          id: "b1",
          source: "bounce",
          category: "customer_waiting",
          level: 3,
          consequence: "buyer_waits",
          title: "Your quote never arrived",
          detail: "The address does not exist at that domain.",
          because: [],
          actions: [],
        },
        {
          id: "u1",
          source: "undelivered",
          category: "customer_waiting",
          level: 3,
          consequence: "buyer_waits",
          title: "A message never left",
          detail: "Held: the recipient has not consented to marketing mail.",
          because: [],
          actions: [],
        },
        {
          id: "g1",
          source: "automation_run",
          category: "system",
          level: 6,
          consequence: "data_drifts",
          because: [],
          actions: [],
          batch: {
            key: "system_incident",
            count: 12,
            cause: "automation_run:01a05500-0000-7000-8000-0000000000a1",
            label: "Notify sales on a new lead",
          },
        },
        {
          id: "c1",
          source: "capture_health",
          category: "system",
          level: 6,
          consequence: "data_drifts",
          kind: "disconnected",
          title: "A mailbox stopped syncing",
          detail: "lena.fischer@margince.test",
          because: [],
          actions: [],
        },
        {
          id: "w1",
          source: "ai_work_health",
          category: "system",
          level: 6,
          consequence: "data_drifts",
          title: "A recap did not generate",
          detail: "Acme Renewal",
          because: [],
          actions: [],
        },
        {
          id: "i1",
          source: "introduction_request",
          category: "decisions",
          level: 5,
          consequence: "work_blocked",
          title: "Katrin asked for an introduction",
          detail: "They asked to be introduced to the buyer at Turbinenbau.",
          because: [],
          actions: [],
        },
      ],
    });
    return (
      <StoryProviders>
        <WorklistScreen />
      </StoryProviders>
    );
  },
};
