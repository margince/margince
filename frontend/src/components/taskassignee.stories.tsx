// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { TaskTerms } from "./taskassignee";

const READER = "00000000-0000-4000-8000-000000000001";

const meta: Meta<typeof TaskTerms> = {
  title: "Patterns/Task terms",
  component: TaskTerms,
};
export default meta;
type Story = StoryObj<typeof meta>;

function draw(task: Parameters<typeof TaskTerms>[0]["task"]) {
  installFetchStub({ "GET /me": meRoute({ activity: ["read"] }) });
  return (
    <StoryProviders>
      <p className="t-caption">
        <TaskTerms task={task} />
      </p>
    </StoryProviders>
  );
}

export const YoursAndDue: Story = {
  render: () =>
    draw({
      assignee_id: READER,
      due_at: "2026-12-06T16:59:59Z",
      due_has_time: false,
    }),
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByText(/^You · Due /),
    ).toBeVisible();
  },
};

export const UnassignedAndUndated: Story = {
  render: () => draw({ assignee_id: null, due_at: null }),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByText(/^Unassigned · /),
    ).toBeVisible();
  },
};
