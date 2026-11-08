// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { HelpPopover } from "./helppopover";

const meta: Meta<typeof HelpPopover> = {
  title: "Components/Overlays and layering/Help popover",
  component: HelpPopover,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof HelpPopover>;

function render() {
  return (
    <p>
      Signing secret{" "}
      <HelpPopover label="About the signing secret">
        <p>Margince signs every delivery with this secret.</p>
      </HelpPopover>
    </p>
  );
}

/** The glyph beside its label, opened. */
export const Opened: Story = {
  render,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: "About the signing secret" }),
    );
    await expect(
      await within(document.body).findByText(
        "Margince signs every delivery with this secret.",
      ),
    ).toBeTruthy();
  },
};

/** The same in the dark theme. */
export const OpenedDark: Story = { ...Opened, globals: { theme: "dark" } };
