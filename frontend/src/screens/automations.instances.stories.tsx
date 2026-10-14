// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
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
type AutomationRun = components["schemas"]["AutomationRun"];

const RENEWAL_RUNS: AutomationRun[] = [
  {
    id: "r-1",
    automation_id: "au-2",
    occurred_at: "2026-07-14T10:00:00Z",
    outcome: "fired",
    tier: "auto_execute",
    trigger_evidence: "renewal date in 30 days",
    action_result: "task created",
  },
  {
    id: "r-2",
    automation_id: "au-2",
    occurred_at: "2026-07-13T10:00:00Z",
    outcome: "failed",
    tier: "auto_execute",
    reason: "the owner has no mailbox connected",
  },
];

const entryFor = (key: string) =>
  AUTOMATION_CATALOG.find((entry) => entry.key === key);

function Table({ canEdit = true }: Readonly<{ canEdit?: boolean }>) {
  installFetchStub({
    "GET /automations/au-1/runs": () =>
      jsonResponse({ data: [], page: { next_cursor: null } }),
    "GET /automations/au-2/runs": () =>
      jsonResponse({ data: RENEWAL_RUNS, page: { next_cursor: null } }),
    "POST /automations/au-1/preview": () =>
      jsonResponse({ matches_now: 8, would_have_fired: 21, window_days: 30 }),
  });
  return (
    <StoryProviders>
      <Panel title="Configured automations">
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
  await expect(canvas.getByText("Needs approval")).toBeVisible();
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

// Folded: name and recipe lead, and the switch and the menu sit at the end.
// Mode, last run and the run count follow as a caption.
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

// The run history opens under its rule as a plain body: an empty one is a
// single line, with no outcome dial over nothing.
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
    await canvas.findByText("This automation has not fired yet.");
    await expect(canvas.queryByRole("combobox")).toBeNull();
  },
  render: () => <Table />,
};

const openRenewalRuns: NonNullable<Story["play"]> = async ({
  canvasElement,
}) => {
  const canvas = within(canvasElement);
  await userEvent.click(
    await canvas.findByRole("button", {
      name: "Run history of Renewal reminder",
    }),
  );
  await canvas.findByText("the owner has no mailbox connected");
};

// A rule that has fired: its runs read down under the row, under an outcome
// dial. The columns above keep their widths.
export const RunHistoryPopulated: Story = {
  play: openRenewalRuns,
  render: () => <Table />,
};

export const RunHistoryPopulatedPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: openRenewalRuns,
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
  render: () => {
    const automation = configuredAutomations(Date.now())[5];
    if (!automation) throw new Error("the fixtures hold no sixth automation");
    return (
      <StoryProviders>
        <RulePausedReason automation={automation} />
      </StoryProviders>
    );
  },
};
