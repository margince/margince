// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../../testing/story-utils";
import { CloseTasksQuestion, useCloseTasks } from "./closetasks";

// The question a deal's close asks about its open tasks: the tasks listed with
// their due days, then Complete all or Keep. The dialog that holds it has its
// own gallery in confirmadvance.stories.tsx.

const OPEN_TASKS = {
  data: [
    {
      id: "t-invoice",
      kind: "task",
      subject: "Send the final invoice",
      is_done: false,
      version: 2,
      occurred_at: "2026-10-01T09:00:00Z",
      due_at: "2026-11-04T16:00:00Z",
    },
    {
      id: "t-device",
      kind: "task",
      subject: "Return the loan device",
      is_done: false,
      version: 1,
      occurred_at: "2026-10-02T09:00:00Z",
    },
  ],
  page: { has_more: false },
};

function Question() {
  const state = useCloseTasks("deal-1");
  return <CloseTasksQuestion state={state} />;
}

const meta: Meta<typeof Question> = {
  title: "Records/Deal 360/Close tasks question",
  component: Question,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof Question>;

export const OpenTasks: Story = {
  render: () => {
    installFetchStub({ "GET /activities": () => jsonResponse(OPEN_TASKS) });
    return (
      <StoryProviders>
        <Question />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const group = await canvas.findByRole("group");
    await expect(within(group).getAllByRole("radio")).toHaveLength(2);
    await expect(canvas.getByText("Send the final invoice")).toBeVisible();
    await userEvent.click(canvas.getByRole("radio", { name: /Keep/ }));
    await expect(canvas.getByRole("radio", { name: /Keep/ })).toBeChecked();
  },
};
