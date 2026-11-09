// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import { StoryProviders } from "../../testing/story-utils";
import { PlanFoot } from "./brief.plan.foot";
import "./brief.plan.css";

// The weekly plan's save bar: the Save that counts what is ticked, and the
// refusal that stays after the week closed.
const meta: Meta<typeof PlanFoot> = {
  title: "Shell/Home/Plan save bar",
  component: PlanFoot,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  args: {
    editable: true,
    staged: 0,
    unsaved: 0,
    saving: false,
    onSave: fn(),
  },
};
export default meta;
type Story = StoryObj<typeof PlanFoot>;

/** Two rows ticked: one Save that names how many. */
export const Staged: Story = {
  args: { staged: 2 },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("button", { name: "Save 2 changes" }),
    ).toBeVisible();
    await expect(canvas.queryByText("Some changes not saved")).toBeNull();
  },
};

export const StagedDark: Story = { ...Staged, globals: { theme: "dark" } };

/** The save is in flight: the button stays, focused, and shows it waits. */
export const Saving: Story = { args: { staged: 2, saving: true } };

/**
 * The week closed under the save. The refusal stays with its count; the
 * button goes, because the rows no longer have boxes to send.
 */
export const RefusedAfterClose: Story = {
  args: { editable: false, staged: 2, unsaved: 2 },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Some changes not saved")).toBeVisible();
    await expect(
      canvas.getByText(
        "2 commitments were not saved. They are still checked. Retry.",
      ),
    ).toBeVisible();
    await expect(canvas.queryByRole("button")).toBeNull();
  },
};

export const RefusedAfterCloseDark: Story = {
  ...RefusedAfterClose,
  globals: { theme: "dark" },
};
