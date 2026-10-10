// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { Panel } from "../design-system/panel";
import {
  AUTOMATION_CATALOG,
  configuredAutomations,
} from "./automations.fixtures";
import { ConfiguredAutomations } from "./automations.instances";
import { RulePausedReason } from "./automations.lists";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

const meta: Meta = {
  title: "Settings/AI/Automations/Configured automations",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const entryFor = (key: string) =>
  AUTOMATION_CATALOG.find((entry) => entry.key === key);

function Table({ canEdit = true }: Readonly<{ canEdit?: boolean }>) {
  installFetchStub({
    "GET /automations/au-1/runs": () =>
      jsonResponse({ data: [], page: { next_cursor: null } }),
    "POST /automations/au-1/preview": () =>
      jsonResponse({ matches_now: 8, would_have_fired: 21, window_days: 30 }),
  });
  return (
    <StoryProviders>
      <Panel title="Automations">
        <ConfiguredAutomations
          automations={configuredAutomations(Date.now())}
          entryFor={entryFor}
          canViewRuns
          canEdit={canEdit}
          canDelete={canEdit}
        />
      </Panel>
    </StoryProviders>
  );
}

const rendersTable: NonNullable<Story["play"]> = async ({ canvasElement }) => {
  const canvas = within(canvasElement);
  await canvas.findByRole("switch", { name: "Renewal reminder is enabled" });
  await expect(canvas.getByText("Never")).toBeVisible();
};

export const Outcomes: Story = {
  play: rendersTable,
  render: () => <Table />,
};

export const OutcomesDark: Story = {
  globals: { theme: "dark" },
  play: rendersTable,
  render: () => <Table />,
};

// Folded: name and recipe lead, the switch and the menu sit at the end, and
// mode, last run and the run count follow as a caption.
export const OutcomesPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: rendersTable,
  render: () => <Table />,
};

// A seat without update reads each rule's state as a word, not a switch.
export const ReadOnly: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("button", {
      name: "Actions for Renewal reminder",
    });
    await expect(canvas.queryByRole("switch")).toBeNull();
  },
  render: () => <Table canEdit={false} />,
};

// The run history opens under the table, headed by the rule it belongs to.
export const RunHistoryOpen: Story = {
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    const canvas = within(canvasElement);
    await user.click(
      await canvas.findByRole("button", {
        name: "Actions for Quiet accounts follow-up",
      }),
    );
    const body = within(canvasElement.ownerDocument.body);
    await user.click(await body.findByRole("button", { name: "Runs" }));
    await canvas.findByRole("heading", { name: "Run history" });
  },
  render: () => <Table />,
};

// A rule the system paused names why in short; a press gives the whole reason.
export const PausedReason: Story = {
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    await user.click(
      await within(canvasElement).findByRole("button", {
        name: "Paused · list archived",
      }),
    );
    await within(canvasElement.ownerDocument.body).findByText(
      /Restoring the list does not resume it/,
    );
  },
  render: () => (
    <StoryProviders>
      <RulePausedReason automation={configuredAutomations(Date.now())[5]} />
    </StoryProviders>
  ),
};
