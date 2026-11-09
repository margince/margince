// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { Card } from "../../design-system/atoms";
import { StoryProviders } from "../../testing/story-utils";
import { CardPager } from "./brief.cardpager";

const meta: Meta<typeof CardPager> = {
  title: "Shell/Home/Task navigation",
  component: CardPager,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof CardPager>;

function TaskStack() {
  const [at, setAt] = useState(0);
  const tasks = [
    "Send the revised quote",
    "Call the buyer",
    "Review the proposal",
  ];
  return (
    <CardPager
      title={tasks[at] ?? ""}
      previous={at > 0 ? () => setAt(at - 1) : undefined}
      next={at + 1 < tasks.length ? () => setAt(at + 1) : undefined}
    >
      <Card as="article">
        <p>{tasks[at]}</p>
      </Card>
    </CardPager>
  );
}
export const SeveralTasks: Story = { render: () => <TaskStack /> };
