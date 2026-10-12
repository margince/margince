// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { AiUsageCard } from "./aiusage";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The card gates itself on ai_diagnostics:read — the grant `GET /ai/usage`
// asks for — so /me is not optional furniture here: it decides which of the
// card's two whole branches renders. A story that leaves /me to the stub's
// list-shaped fallback gets a body with no `user`, which useMe rejects as
// malformed, which fails every grant closed.
// The five band/state stories below were all drawing that one probe-error
// branch, under five names that each promised something else.
const OPERATOR: GrantSpec = { ai_diagnostics: ["read"] };

function story(
  band: string,
  tasks: Record<string, unknown>[],
  allow: GrantSpec = OPERATOR,
) {
  return () => {
    installFetchStub({
      "GET /me": () => jsonResponse(meFixture({ allow })),
      "GET /ai/usage": () =>
        jsonResponse({
          days: tasks.length ? [{ date: "2026-07-20", tasks }] : [],
          budget: {
            monthly_tokens: 1000,
            spent_tokens:
              band === "queued" ? 1000 : band === "degraded" ? 850 : 200,
            band,
            currency: "EUR",
          },
        }),
    });
    return (
      <StoryProviders>
        <AiUsageCard />
      </StoryProviders>
    );
  };
}

const task = {
  task: "capture_classify",
  tier: "cheap_cloud",
  calls: 8,
  cached_hits: 2,
  tokens_in: 1200,
  tokens_out: 240,
};
const meta: Meta<typeof AiUsageCard> = {
  title: "Settings/AI/AI usage/Usage",
  component: AiUsageCard,
};
export default meta;
type Story = StoryObj<typeof AiUsageCard>;
export const Normal: Story = { render: story("normal", [task]) };
export const EconomyMode: Story = { render: story("degraded", [task]) };
export const Queued: Story = { render: story("queued", [task]) };
export const WithCost: Story = {
  render: story("normal", [{ ...task, cost_est_minor: 124 }]),
};
export const Empty: Story = { render: story("normal", []) };

// A seat holding no ai_diagnostics grant. The card keeps its place and says the
// figures are withheld — an absent spend card would read as "this
// installation meters nothing", a claim about the data rather than about who
// may read it.
export const Withheld: Story = { render: story("normal", [task], {}) };

// The per-day breakdown, opened: the card's diagnostic half, a small table of
// days and their calls behind its own summary.
export const DaysOpen: Story = {
  render: story("normal", [task]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("columnheader", { name: "Task" });
    await userEvent.click(canvas.getByText("Show days"));
    await canvas.findByRole("columnheader", { name: "Day" });
  },
};

const grouped = [
  { ...task, cost_est_minor: 312 },
  { ...task, tier: "premium", calls: 3, tokens_in: 9400, cost_est_minor: 1890 },
  {
    task: "weekly_review",
    task_display_name: "Weekly review narrative",
    tier: "premium",
    calls: 4,
    tokens_in: 3676,
    tokens_out: 1200,
    cost_est_minor: 840,
  },
];
export const GroupedByTask: Story = { render: story("normal", grouped) };
export const GroupedByTaskDark: Story = {
  globals: { theme: "dark" },
  render: story("normal", grouped),
};
export const GroupedByTaskPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story("normal", grouped),
};

// Economy mode in dark. The band is carried twice and both times by colour: the
// Badge tone and the Meter's fill at 85% of budget. Nothing else on the card
// says spend has crossed into throttling, so if either tint flattens against
// the dark panel the reader sees an ordinary month.
export const EconomyModeDark: Story = {
  globals: { theme: "dark" },
  render: story("degraded", [task]),
};

// The widest the table gets, at 390px. No spend row is reconcilable in pieces,
// so the table scrolls inside the card with the task column pinned.
export const WithCostPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story("normal", [{ ...task, cost_est_minor: 124 }]),
};
