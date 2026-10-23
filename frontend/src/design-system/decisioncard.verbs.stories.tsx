// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { createRef } from "react";
import { expect, within } from "storybook/test";
import type { DecisionCardLabels } from "./decisioncard";
import { DecisionAnswers } from "./decisioncard.verbs";

// The card's answer row on its own. The assembled card, and the held rejection
// a reader reaches by pressing Reject, are under `Decision card/Card`.
const meta: Meta<typeof DecisionAnswers> = {
  title: "Components/AI and provenance/Decision card/Verbs",
  component: DecisionAnswers,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div className="staging-card dcard" data-layout="deck">
        <Story />
      </div>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof DecisionAnswers>;

const LABELS: DecisionCardLabels = {
  accept: "Accept",
  edit: "Edit",
  reject: "Reject",
  skip: "Later",
  expired: "This ran out of time before anyone answered it.",
  draftSubject: "Subject",
  draftBody: "Message",
  noContent: "This proposal carries nothing to read.",
  loading: "Reading the proposal",
};

export const Full: Story = {
  args: {
    decided: false,
    answering: true,
    focusNextRef: createRef(),
    labels: LABELS,
    onAccept: () => undefined,
    onEdit: () => undefined,
    onReject: () => undefined,
    onSkip: () => undefined,
  },
  play: async ({ canvasElement }) => {
    const reject = await within(canvasElement).findByRole("button", {
      name: "Reject",
    });
    await expect(reject).toHaveTextContent("Reject");
  },
};

export const Compact: Story = {
  args: { ...Full.args, compact: { detail: "Proposal", more: "More options" } },
};

export const Pending: Story = {
  args: { ...Full.args, pending: true },
};

export const Held: Story = {
  args: {
    ...Full.args,
    held: { label: "Rejected", undo: "Undo", onUndo: () => undefined },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("Rejected")).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Undo" })).toBeVisible();
  },
};
