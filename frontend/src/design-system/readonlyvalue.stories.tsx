// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { ReadOnlyValue } from "./readonlyvalue";

const meta = {
  title: "Components/Overlays and layering/Read-only value",
  component: ReadOnlyValue,
  parameters: { layout: "padded" },
  args: { children: "Customer" },
} satisfies Meta<typeof ReadOnlyValue>;
export default meta;
type Story = StoryObj<typeof meta>;

const REASON = "This company is archived. Restore it to change its stage.";

/** The reason, raised from the keyboard: the value takes a tab stop. */
export const WithReason: Story = {
  args: { reason: REASON },
  play: async ({ canvasElement }) => {
    const value = within(canvasElement).getByText("Customer");
    await expect(value).toHaveAccessibleDescription(REASON);
    await userEvent.tab();
    await expect(value).toHaveFocus();
    await expect(
      await within(document.body).findByRole("tooltip"),
    ).toHaveTextContent(REASON);
  },
};

/** No reason: plain text, and no tab stop. */
export const WithoutReason: Story = {
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByText("Customer"),
    ).not.toHaveAttribute("tabindex");
  },
};

/** The reason in the dark theme. */
export const WithReasonDark: Story = {
  ...WithReason,
  globals: { theme: "dark" },
};
