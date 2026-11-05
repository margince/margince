// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import { company360 } from "../../testing/company.fixtures";
import { StoryProviders } from "../../testing/story-utils";
import { ProposedNextSteps } from "./company360.suggestions";

// The advice that would become a task, drawn where the tasks are. Each row is
// dated by the evidence it fired on, never by a deadline the system chose.

type Suggestion = components["schemas"]["Company360Suggestion"];

const STEP = 'Agree the next step on "PIM rollout Phase 2"';

const step: Suggestion = {
  kind: "no_next_step",
  fingerprint: "f1",
  title: STEP,
  reason: '"PIM rollout Phase 2" is open and no task says what happens next.',
  evidence: [{ entity_type: "deal", entity_id: "d-1" }],
  action: {
    kind: "add_task",
    deal_id: "d-1",
    task: {
      subject: STEP,
      source: "manual",
      links: [{ entity_type: "deal", entity_id: "d-1" }],
    },
  },
};

function Steps({ suggestion }: Readonly<{ suggestion: Suggestion }>) {
  return (
    <ProposedNextSteps
      companyId={company360.company.id}
      view={{
        ...company360,
        suggestions: [suggestion],
        suggestions_dropped: 0,
      }}
      onOpenReceipt={() => undefined}
    />
  );
}

const meta = {
  title: "Records/Company 360/Proposed next steps",
  component: Steps,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof Steps>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Undated: Story = { args: { suggestion: step } };

// The evidence is a call logged by its date alone. The row names that date and
// no clock, so its stored noon never reads as the hour it happened.
export const DatedByADayEntry: Story = {
  args: {
    suggestion: {
      ...step,
      due_at: "2026-08-12T12:00:00Z",
      due_precision: "day",
    },
  },
  play: async ({ canvasElement }) => {
    const panel = await within(canvasElement).findByText(STEP);
    const row = panel.closest("li") ?? canvasElement;
    await expect(row.textContent).toMatch(/12/);
    await expect(row.textContent).not.toMatch(/\d:\d\d/);
  },
};
