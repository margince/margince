// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { MoveButton } from "./movebutton";

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

/** A seat that may not write activities is offered no follow-up. */
export const CreateTaskNoGrant: Story = {
  beforeEach: () => {
    installFetchStub({ "GET /me": meRoute({ activity: ["read"] }) });
  },
  args: CREATE_TASK,
  play: async () => {
    // Drawn while the grants are unknown, gone once /me refuses the write.
    await screen.findByRole("button", { name: "Create follow-up" });
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Create follow-up" }),
      ).toBeNull(),
    );
  },
};

/** A seat that may not write activities is offered no follow-up draft. */
export const DraftFollowUpNoGrant: Story = {
  beforeEach: () => {
    installFetchStub({ "GET /me": meRoute({ activity: ["read"] }) });
  },
  args: {
    dealId: "d-1",
    move: {
      action: "draft_email",
      arguments: {
        activity_id: "m-1",
        links: [{ entity_type: "contact", entity_id: "p-1" }],
      },
    },
  },
  play: async () => {
    await screen.findByRole("button", { name: "Draft follow-up" });
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Draft follow-up" }),
      ).toBeNull(),
    );
  },
};

export const RecommendationChanged: Story = {
  beforeEach: () => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read", "create"] }),
      "GET /deals/d-1/status": () =>
        jsonResponse(
          {
            type: "about:blank",
            title: "Suggestion changed",
            status: 409,
            detail:
              "The suggested step changed. Refresh the record before opening it.",
          },
          409,
        ),
    });
  },
  args: {
    dealId: "d-1",
    move: {
      action: "open_task",
      arguments: { activity_id: "task-1" },
      recommendation: {
        deal_id: "d-1",
        rule: "open_task",
        policy_version: "fixture",
        basis_fingerprint: "a".repeat(64),
        checked_at: "2026-10-12T09:00:00Z",
      },
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Open task" }),
    );
    await expect(await canvas.findByRole("alert")).toHaveTextContent(
      "The suggested step changed.",
    );
  },
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
      await canvas.findByRole("button", { name: "Create follow-up" }),
    );
    await expect(await canvas.findByRole("alert")).toHaveTextContent(
      "The deal this task names is archived.",
    );
  },
};

// The press wrote the task. The verb gives way to a line naming it, with the
// way into it, so the reader sees what they made.
export const CreateTaskCreated: Story = {
  beforeEach: () => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read", "create", "update"] }),
      "POST /tasks": () =>
        jsonResponse(
          { id: "task-9", subject: "Send the revised offer", deal_id: "d-1" },
          201,
        ),
    });
  },
  args: CREATE_TASK,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Create follow-up" }),
    );
    await expect(await canvas.findByRole("status")).toHaveTextContent(
      "Follow-up created",
    );
    await expect(
      canvas.getByRole("button", { name: "Open task" }),
    ).toBeVisible();
    await expect(
      canvas.queryByRole("button", { name: "Create follow-up" }),
    ).toBeNull();
  },
};

export const CreateTaskCreatedDark: Story = {
  ...CreateTaskCreated,
  globals: { theme: "dark" },
};

export const RecommendationChangedDark: Story = {
  ...RecommendationChanged,
  globals: { theme: "dark" },
};
