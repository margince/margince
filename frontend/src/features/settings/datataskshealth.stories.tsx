// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import type { GrantSpec } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { DataTasksHealthCard } from "./datataskshealth";
import { runningDataTask } from "./datataskshealth.fixtures";

type Progress = components["schemas"]["DataTaskProgress"];
function report(
  rows: Progress[],
  grants: GrantSpec = { data_task: ["read", "update"] },
  status = 200,
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(grants),
      "GET /admin/data-tasks": () =>
        jsonResponse(
          status === 200
            ? { data: rows }
            : { code: "internal", message: "The report could not be read." },
          status,
        ),
      "POST /admin/data-tasks/control": () =>
        new Response(null, { status: 204 }),
    });
    return (
      <StoryProviders>
        <DataTasksHealthCard />
      </StoryProviders>
    );
  };
}
const meta = {
  title: "Settings/Governance/System health/Data updates",
  component: DataTasksHealthCard,
} satisfies Meta<typeof DataTasksHealthCard>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Running: Story = {
  render: report([runningDataTask]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("Not estimated")).toBeInTheDocument();
    await expect(
      canvas.getByRole("button", { name: "Pause" }),
    ).toBeInTheDocument();
  },
};
export const CappedEstimate: Story = {
  render: report([
    { ...runningDataTask, estimate: 1000, estimate_capped: true },
  ]),
};
export const Empty: Story = {
  render: report([]),
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByText("No data updates are registered."),
    ).toBeInTheDocument();
  },
};
export const FailedTask: Story = {
  render: report([
    {
      ...runningDataTask,
      status: "failed",
      blocked_reason: "failed",
      error_sentence: "The update stopped after repeated timeouts.",
      error_remedy: "Check the worker before retrying.",
    },
  ]),
};
export const Paused: Story = {
  render: report([
    { ...runningDataTask, status: "paused", blocked_reason: "paused" },
  ]),
};
export const WaitingForRelease: Story = {
  render: report([
    {
      ...runningDataTask,
      status: "pending",
      blocked_reason: "newer_release_required",
      first_release: "2.0",
    },
  ]),
};
export const ReadOnly: Story = {
  render: report([runningDataTask], { data_task: ["read"] }),
};
export const Withheld: Story = { render: report([], {}) };
export const FailedReport: Story = {
  render: report([], { data_task: ["read"] }, 500),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByRole("alert")).toBeInTheDocument();
    await expect(
      canvas.queryByText("No data updates are registered."),
    ).toBeNull();
  },
};

export const NewerServerState: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ data_task: ["read", "update"] }),
      "GET /admin/data-tasks": () =>
        jsonResponse({
          data: [
            runningDataTask,
            {
              ...runningDataTask,
              task_id: "new-task",
              title: "Newer data update",
              status: "awaiting_new_release",
            },
          ],
        }),
    });
    return (
      <StoryProviders>
        <DataTasksHealthCard />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByText("Update the application to manage this state."),
    ).toBeInTheDocument();
    await expect(canvas.getAllByRole("button", { name: "Pause" })).toHaveLength(
      1,
    );
  },
};
