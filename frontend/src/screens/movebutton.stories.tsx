// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { MoveButton } from "./movebutton";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

const meta: Meta<typeof MoveButton> = {
  title: "Patterns/Move button",
  component: MoveButton,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof MoveButton>;

const CREATE_TASK = {
  dealId: "d-1",
  move: {
    action: "create_task",
    arguments: { subject: "Send the revised offer", deal_id: "d-1" },
  },
};

export const CreateTask: Story = {
  beforeEach: () => {
    installFetchStub({ "GET /me": meRoute({ activity: ["read", "create"] }) });
  },
  args: CREATE_TASK,
};

// The server refused the task the rule prepared; the refusal sits under the
// verb that asked for it.
export const CreateTaskRefused: Story = {
  beforeEach: () => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read", "create"] }),
      "POST /tasks": () =>
        jsonResponse(
          {
            type: "about:blank",
            title: "Task refused",
            status: 422,
            detail: "The deal this task names is archived.",
          },
          422,
        ),
    });
  },
  args: CREATE_TASK,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Add this task" }),
    );
    await expect(await canvas.findByRole("alert")).toHaveTextContent(
      "The deal this task names is archived.",
    );
  },
};
