// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { StoryProviders } from "../../testing/story-utils";
import type { Worklist } from "./worklist.queries";
import { WorklistViewPicker } from "./worklist.views";

// The three views with their counts. A view draws a number only when its count
// is exact, so the second frame shows Decisions without one.

type Count = Worklist["counts"][number];

function day(counts: Count[]): Worklist {
  return {
    as_of: "2026-09-02T09:00:00Z",
    scope: "mine",
    scope_options: ["mine"],
    queue: [],
    summary: { urgent: 1, due: 2, in_play: 0, lower_priority: 1, total: 12 },
    sources_unavailable: [],
    reach: [],
    counts,
    readings: {
      changed_since_brief: 0,
      revenue_at_risk_minor: 0,
      revenue_currency: "EUR",
      buyer_replies: 0,
      prospecting: 0,
      review: 0,
      more_available: false,
    },
  };
}

const exact = (category: Count["category"], considered: number): Count => ({
  category,
  considered,
  shown: considered,
  more_available: false,
});

const counted = [
  exact("tasks", 9),
  exact("customer_waiting", 3),
  exact("decisions", 4),
  exact("system", 2),
];

const meta: Meta<typeof WorklistViewPicker> = {
  title: "Records/Worklist/Views",
  component: WorklistViewPicker,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof WorklistViewPicker>;

export const EveryCountExact: Story = {
  render: () => (
    <StoryProviders>
      <WorklistViewPicker day={day(counted)} view="today" onView={() => {}} />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    const picker = within(canvasElement).getByRole("group", {
      name: "Work view",
    });
    await expect(picker).toHaveTextContent("12");
  },
};

export const DecisionsAFloor: Story = {
  render: () => (
    <StoryProviders>
      <WorklistViewPicker
        day={day([
          ...counted.filter((count) => count.category !== "decisions"),
          {
            category: "decisions",
            considered: 200,
            shown: 25,
            more_available: true,
          },
        ])}
        view="decisions"
        onView={() => {}}
      />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    const picker = within(canvasElement).getByRole("group", {
      name: "Work view",
    });
    await expect(picker).not.toHaveTextContent("200");
  },
};
