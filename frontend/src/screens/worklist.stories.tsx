import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { jsonResponse, StoryProviders } from "./story-utils";
import { WorklistScreen } from "./worklist";

// The ranked queue, in the three readings that matter: a full day, a clear one,
// and one the page could not read the whole of.

type Worklist = components["schemas"]["Worklist"];

function stubDay(day: Worklist) {
  globalThis.fetch = (async (input: RequestInfo | URL): Promise<Response> => {
    const url = String(input instanceof Request ? input.url : input);
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
      scope_options: ["mine", "team", "all"],
      summary: { urgent: 1, due: 2, lower_priority: 3, total: 5 },
      sources_unavailable: [],
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
    });
    return (
      <StoryProviders>
        <WorklistScreen />
      </StoryProviders>
    );
  },
};
