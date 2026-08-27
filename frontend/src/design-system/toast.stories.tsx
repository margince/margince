// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../i18n";
import { Button } from "./atoms";
import { ToastRegion, useToast } from "./toast";

const meta: Meta<typeof ToastRegion> = {
  title: "Design System/Toast",
  component: ToastRegion,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof ToastRegion>;

/**
 * Live, because the region's whole subject is timing: a confirmation withdraws
 * itself and a sticky one does not, and neither is visible in a static frame.
 */
function Bench({
  label,
  message,
  mark,
  sticky,
}: Readonly<{
  label: string;
  message: string;
  mark?: boolean;
  sticky?: boolean;
}>) {
  const toast = useToast();
  return (
    <>
      <Button onClick={() => toast.show(message, { mark, sticky })}>
        {label}
      </Button>
      <ToastRegion toast={toast} />
    </>
  );
}

/**
 * A completion. The green dot belongs to the MESSAGE rather than to the region:
 * the same region carries a save that worked and a save that was refused, and a
 * tick beside a refusal says the opposite of what the sentence says.
 */
export const Completion: Story = {
  render: () => (
    <Bench label="Save" message="Lead qualified." mark sticky={false} />
  ),
};

/** A refusal takes no mark — the sentence is the whole signal. */
export const Refusal: Story = {
  render: () => (
    <Bench
      label="Save without a stage"
      message="A deal needs a stage before it can be saved."
      mark={false}
    />
  ),
};

/**
 * Sticky, which is what a toast carrying an action needs: a reader reaching for
 * Undo must not lose it mid-reach. Because it does not withdraw itself it has
 * to offer a way out, and the dismiss button appears only on this arm.
 */
export const StickyWithAWayOut: Story = {
  render: () => <Bench label="Archive" message="Deal archived." mark sticky />,
};
