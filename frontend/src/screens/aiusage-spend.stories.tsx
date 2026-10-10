// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { Panel } from "../design-system/panel";
import { aggregate, CallsByDay, SpendByTask, spendRows } from "./aiusage-spend";
import { StoryProviders } from "./story-utils";

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
    ],
  },
  { date: "2026-07-21", tasks: [line("capture_classify", "decide", 30, 0)] },
];

function Spend({ showCost }: Readonly<{ showCost: boolean }>) {
  return (
    <StoryProviders>
      <Panel title="Estimated AI spend and usage">
        <SpendByTask
          rows={spendRows(aggregate(DAYS))}
          showCost={showCost}
          currency="EUR"
          note={
            showCost
              ? "Costs are estimates at configured rates. €8.18"
              : undefined
          }
        />
        <CallsByDay days={DAYS} />
      </Panel>
    </StoryProviders>
  );
}

const meta: Meta<typeof Spend> = {
  title: "Settings/AI/AI usage/Spend by task",
  component: Spend,
  args: { showCost: true },
};
export default meta;
type Story = StoryObj<typeof Spend>;

export const Grouped: Story = {};
export const Unpriced: Story = { args: { showCost: false } };
export const GroupedDark: Story = { globals: { theme: "dark" } };
export const GroupedPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
export const DaysOpen: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("columnheader", { name: "Task" });
    await userEvent.click(canvas.getByText("Show days"));
    await canvas.findByRole("columnheader", { name: "Day" });
  },
};
