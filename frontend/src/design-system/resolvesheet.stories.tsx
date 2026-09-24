// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { LocaleProvider } from "../i18n";
import { ProblemError } from "../screens/common";
import { Button } from "./atoms";
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
  outcomeLegend: "Answer type",
  outcomes: [
    {
      value: "fixed_record",
      label: "Record corrected",
      description: "The date moved to what the buyer confirmed.",
    },
    { value: "added_evidence", label: "Evidence added" },
    {
      value: "value_correct",
      label: "Value is correct",
      description: "Hides this check until the expiry.",
    },
    {
      value: "not_relevant",
      label: "Not relevant to this deal",
      description: "Hides this check until the expiry.",
    },
    { value: "remind_later", label: "Not now" },
    { value: "reassign", label: "Someone else’s to answer" },
  ],
  reason: "Why",
  reasonHelp:
    "The next person to see this number needs the reason it is not flagged.",
  remindAt: "Remind on",
  expiresAt: "Expires on",
  expiresHelp: "At most 90 days: a value correct in May describes May.",
  cancel: "Cancel",
  submit: "Save answer",
};

const noop = () => {};

/**
 * The sheet with the control that opens it, the way `atoms.stories.tsx` frames
 * a dialog.
 *
 * The trigger is not decoration: `Modal` portals to the document body, so a
 * frame whose only content is the sheet leaves the story's own root EMPTY —
 * which reads to a render gate as a component that drew nothing. It is also
 * what lets a reader reopen the drawer after dismissing it.
 */
function ResolveSheetDemo({
  pending = false,
  error,
}: Readonly<{ pending?: boolean; error?: unknown }>) {
  const [open, setOpen] = useState(true);
  return (
    <>
      <Button variant="primary" onClick={() => setOpen(true)}>
        Answer the finding
      </Button>
      <ResolveSheet
        open={open}
        pending={pending}
        error={error}
        labels={labels}
        onSubmit={noop}
        onClose={() => setOpen(false)}
      />
    </>
  );
}

export const Open: Story = {
  render: () => <ResolveSheetDemo />,
};

// The state a save is in flight from: the control is out of reach rather than
// gone, so the sheet does not jump under the hand that pressed it.
export const Saving: Story = {
  render: () => <ResolveSheetDemo pending />,
};

// A save the server refused: the sheet stays open with the server's reason
// above the actions, so the answer typed is not lost to a silent failure.
export const Refused: Story = {
  render: () => (
    <ResolveSheetDemo
      error={
        new ProblemError({
          status: 409,
          title: "Conflict",
          detail: "This check was already answered by someone else.",
        })
      }
    />
  ),
};
