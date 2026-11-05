// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { AiUsageCard } from "./aiusage";

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
const meta = {
  title: "Settings/AI/AI usage/Usage",
  component: AiUsageCard,
} satisfies Meta<typeof AiUsageCard>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Normal: Story = { render: story("normal", [task]) };
export const EconomyMode: Story = { render: story("degraded", [task]) };
export const Queued: Story = { render: story("queued", [task]) };
export const WithCost: Story = {
  render: story("normal", [{ ...task, cost_est_minor: 124 }]),
};
export const Empty: Story = { render: story("normal", []) };

// A reader who may read the model prices is pointed to them.
export const WithPricesLink: Story = {
  render: story("normal", [task], { ...OPERATOR, ai_model_rate: ["read"] }),
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByRole("link", {
        name: "Prices are set on AI models",
      }),
    ).toHaveAttribute("href", "/#/settings/models");
  },
};

// A seat holding no ai_diagnostics grant. The card keeps its place and says the
// figures are withheld — an absent spend card would read as "this
// installation meters nothing", a claim about the data rather than about who
// may read it.
export const Withheld: Story = { render: story("normal", [task], {}) };

// The same table by day, switched from the keyboard: the switch is a pair of
// pressed buttons, so Tab reaches it and Enter turns it.
export const ByDay: Story = {
  render: story("normal", [{ ...task, cost_est_minor: 124 }]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("columnheader", { name: "Task" });
    canvas.getByRole("button", { name: "By day" }).focus();
    await userEvent.keyboard("{Enter}");
    await canvas.findByRole("columnheader", { name: "Day" });
  },
};

// Calls with usage and no rate: the total is short and the line under the
// strip says by how many calls.
export const PartlyPriced: Story = {
  render: story("normal", [
    { ...task, cost_est_minor: 124, unpriced_calls: 3 },
  ]),
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
