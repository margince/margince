// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../i18n";
import { ResolveSheet, type ResolveSheetLabels } from "./resolvesheet";

// Answering a finding from the nightly input check.
const meta: Meta = {
  title: "Design System/ResolveSheet",
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj;

const labels: ResolveSheetLabels = {
  title: "Acme — close date may be wrong",
  outcomeLegend: "What kind of answer is this?",
  outcomes: [
    {
      value: "fixed_record",
      label: "I corrected the record",
      description: "The date moved to what the buyer confirmed.",
    },
    { value: "added_evidence", label: "I added the evidence" },
    {
      value: "value_correct",
      label: "The value is correct",
      description: "Hides this check until the expiry.",
    },
    {
      value: "not_relevant",
      label: "Not relevant to this deal",
      description: "Hides this check until the expiry.",
    },
    { value: "remind_later", label: "Not now" },
    { value: "reassign", label: "Somebody else's to answer" },
  ],
  reason: "Why",
  reasonHelp:
    "The next person to see this number is owed the reason it is not flagged.",
  remindAt: "Bring it back on",
  expiresAt: "Stops holding on",
  expiresHelp:
    "At most 90 days: a value that was correct in May is a claim about May.",
  cancel: "Cancel",
  submit: "Save answer",
};

const noop = () => {};

export const Open: Story = {
  render: () => (
    <ResolveSheet open labels={labels} onSubmit={noop} onClose={noop} />
  ),
};

// The state a save is in flight from: the control is out of reach rather than
// gone, so the sheet does not jump under the hand that pressed it.
export const Saving: Story = {
  render: () => (
    <ResolveSheet open pending labels={labels} onSubmit={noop} onClose={noop} />
  ),
};
