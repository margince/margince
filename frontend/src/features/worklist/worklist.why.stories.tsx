// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { en } from "../../i18n/en";
import { StoryProviders, stubWithSession } from "../../testing/story-utils";
import { row } from "./worklist.fixtures";
import type { WorklistItem } from "./worklist.queries";
import { WorklistWhy } from "./worklist.why";

// A row's Details: what happened, why now, what waiting costs, what to do.
const meta: Meta<typeof WorklistWhy> = {
  title: "Records/Worklist/Why",
  component: WorklistWhy,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => {
      stubWithSession({}, { activity: ["read", "update"] });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
};
export default meta;
type Story = StoryObj<typeof WorklistWhy>;

const ALICE = {
  id: "0198c4f1-2b6a-7c3d-9e0f-00000000a11c",
  label: "Alice Müller",
  touch: {
    last_inbound_at: "2026-11-08T09:52:00Z",
    last_inbound_precision: "instant" as const,
    last_outbound_at: "2026-10-29T09:52:00Z",
    last_outbound_precision: "instant" as const,
  },
};

// Two facts, and two reasons that only rank the row against the queue.
const TASK: WorklistItem = row({
  id: "task-1",
  level: 2,
  due_at: "2026-11-10T16:00:00Z",
  because: [
    { kind: "due_today" },
    { kind: "material" },
    { kind: "unassigned" },
    { kind: "routine" },
  ],
  contact: ALICE,
});

/** The due day is told once; the rank comparisons are left out. */
export const Task: Story = {
  args: { item: TASK },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText(en["worklist.why.title"])).toBeVisible();
    await expect(canvas.getByText(en["worklist.why.happened"])).toBeVisible();
    await expect(
      canvas.getByText(/^Last contact from Alice Müller:/),
    ).toBeVisible();
    const reasons = canvas.getByRole("list", {
      name: en["worklist.why.reasons"],
    });
    await expect(
      within(reasons)
        .getAllByRole("listitem")
        .map((reason) => reason.textContent),
    ).toEqual([en["worklist.because.unassigned"]]);
    await expect(
      canvas.getByText(en["worklist.consequence.task_slips"]),
    ).toBeVisible();
    await expect(canvas.getByText(en["worklist.todo.task"])).toBeVisible();
  },
};

export const TaskDark: Story = {
  ...Task,
  globals: { theme: "dark" },
};

/** A customer who wrote, by name and day, and who keeps waiting. */
export const CustomerWaiting: Story = {
  args: {
    item: row({
      id: "wait-1",
      source: "customer_waiting",
      category: "customer_waiting",
      level: 1,
      consequence: "buyer_waits",
      occurred_at: "2026-11-07T08:15:00Z",
      because: [
        { kind: "buyer_wrote_last" },
        { kind: "waiting_days", value: { kind: "days", days: 3 } },
      ],
      contact: ALICE,
    }),
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText(/^Alice Müller wrote on .*, and nobody has answered/),
    ).toBeVisible();
    await expect(
      canvas.getByText("Alice Müller keeps waiting for an answer."),
    ).toBeVisible();
    await expect(
      canvas.getByText(en["worklist.todo.customer_waiting"]),
    ).toBeVisible();
  },
};

/** A deal drifting: its figures, and the deal named in what waiting costs. */
export const DealAtRisk: Story = {
  args: {
    item: row({
      id: "deal-1",
      source: "deal_at_risk",
      category: "deals_at_risk",
      level: 3,
      consequence: "deal_drifts",
      subject: {
        type: "deal",
        id: "0198c4f1-2b6a-7c3d-9e0f-0000000dea11",
        label: "Depot rollout",
      },
      deal: {
        amount_minor: 5_800_000,
        currency: "EUR",
        expected_close_date: "2026-12-15",
      },
      because: [
        { kind: "quiet_days", value: { kind: "days", days: 18 } },
        { kind: "no_next_step" },
      ],
    }),
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText(/^Depot rollout: €58,000/)).toBeVisible();
    await expect(
      canvas.getByText("Depot rollout keeps drifting without a next step."),
    ).toBeVisible();
    // The figures say what happened, so the ask keeps only its next step.
    await expect(
      canvas.getByText(en["worklist.todo.deal_at_risk"]),
    ).toBeVisible();
  },
};

/** A stopped automation, named, with the engine's reason as its detail. */
export const AutomationStopped: Story = {
  args: {
    item: row({
      id: "run-1",
      source: "automation_run",
      category: "system",
      level: 2,
      consequence: "you_believe_it_happened",
      occurred_at: "2026-11-09T06:00:00Z",
      cause_label: "Welcome sequence",
      detail: "The mailbox it sends from is disconnected.",
      because: [{ kind: "repeated_failure" }],
    }),
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText(/^The automation Welcome sequence stopped on /),
    ).toBeVisible();
    await expect(
      canvas.getByText(en["worklist.consequence.you_believe_it_happened"]),
    ).toBeVisible();
  },
};

/** Only rank comparisons and no facts: no list, and the ask still stands. */
export const OnlyRanked: Story = {
  args: {
    item: row({
      source: "dedupe_candidate",
      category: "decisions",
      consequence: "data_drifts",
      because: [{ kind: "material" }, { kind: "routine" }],
    }),
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText(en["worklist.ask.dedupe_candidate"]),
    ).toBeVisible();
    await expect(
      canvas.queryByRole("list", { name: en["worklist.why.reasons"] }),
    ).toBeNull();
    await expect(canvas.queryByText(en["worklist.why.happened"])).toBeNull();
  },
};
