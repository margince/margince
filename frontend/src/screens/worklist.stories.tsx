import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
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

// The notice's one verb: settled here, not linked away.
function stubNoticeDay(readPending: boolean) {
  const day: Worklist = {
    as_of: "2026-08-31T09:00:00Z",
    scope: "mine",
    scope_options: ["mine"],
    summary: { urgent: 0, due: 1, lower_priority: 0, total: 1 },
    sources_unavailable: [],
    reach: [],
    counts: [],
    queue: [
      {
        id: "n1",
        source: "notice",
        category: "tasks",
        level: 4,
        consequence: "task_slips",
        title: "A deal you own changed stage",
        detail: "Acme Renewal moved to a new pipeline stage.",
        because: [],
        actions: ["acknowledge"],
      },
    ],
  };
  globalThis.fetch = (async (input: RequestInfo | URL): Promise<Response> => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.includes("/notices/n1/read")) {
      return readPending
        ? new Promise(() => {})
        : new Response(null, { status: 204 });
    }
    if (url.includes("/worklist")) {
      return jsonResponse(day);
    }
    return jsonResponse({ data: [] });
  }) as typeof fetch;
}

export const ANoticeToSettle: Story = {
  render: () => {
    stubNoticeDay(false);
    return (
      <StoryProviders>
        <WorklistScreen />
      </StoryProviders>
    );
  },
};

// Pending: "Got it" is clicked and the read call never resolves.
export const ANoticeSettling: Story = {
  render: () => {
    stubNoticeDay(true);
    return (
      <StoryProviders>
        <WorklistScreen />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Got it" }),
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
