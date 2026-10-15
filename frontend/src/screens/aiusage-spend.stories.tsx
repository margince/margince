// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { within } from "storybook/test";
import { Panel } from "../design-system/panel";
import { StoryProviders } from "../testing/story-utils";
import { aggregate, SpendByDay, SpendByTask, spendRows } from "./aiusage-spend";

const line = (
  task: string,
  tier: string,
  calls: number,
  cost_est_minor?: number,
) => ({
  task,
  task_display_name:
    task === "owed_verdict" ? "Unanswered-message triage" : undefined,
  tier,
  calls,
  cached_hits: 1,
  tokens_in: calls * 920,
  tokens_out: calls * 40,
  cost_est_minor,
});

const DAYS = [
  {
    date: "2026-07-20",
    tasks: [
      line("owed_verdict", "cheap_cloud", 12, 48),
      line("owed_verdict", "premium", 2, 210),
      line("weekly_review", "premium", 4, 560),
      line("weekly_review", "cheap_cloud", 3),
    ],
  },
  { date: "2026-07-21", tasks: [line("capture_classify", "decide", 30, 0)] },
];

function Spend({
  showCost,
  view,
}: Readonly<{ showCost: boolean; view: "task" | "day" }>) {
  return (
    <StoryProviders>
      <Panel title="AI spend">
        {view === "task" ? (
          <SpendByTask
            rows={spendRows(aggregate(DAYS))}
            showCost={showCost}
            currency="EUR"
          />
        ) : (
          <SpendByDay days={DAYS} showCost={showCost} currency="EUR" />
        )}
      </Panel>
    </StoryProviders>
  );
}

const meta = {
  title: "Settings/AI/AI usage/Spend by task",
  component: Spend,
  args: { showCost: true, view: "task" },
} satisfies Meta<typeof Spend>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Grouped: Story = {};
export const Unpriced: Story = { args: { showCost: false, view: "task" } };
export const GroupedDark: Story = { globals: { theme: "dark" } };
export const GroupedPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
// One line per day, summed over its tasks, under the same figure columns.
export const ByDay: Story = {
  args: { showCost: true, view: "day" },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("columnheader", { name: "Day" });
    await canvas.findByRole("columnheader", { name: "Estimated cost" });
  },
};
