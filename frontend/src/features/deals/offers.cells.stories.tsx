// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, userEvent } from "storybook/test";
import { useLocale, useT } from "../../i18n";
import { StoryProviders } from "../../testing/story-utils";
import { discountFault } from "./offerlinerates";
import { NumericCell } from "./offers.cells";
import "./offers.css";

type Args = Readonly<{ onSave: (num: number) => Promise<unknown> }>;

// A line's discount cell, holding the same bounds the add form checks.
function DiscountCell({ onSave }: Args) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <NumericCell
      value={10}
      width={90}
      testId="line-discount"
      check={(text) => discountFault(text, locale, t)}
      onSave={onSave}
    />
  );
}

const meta: Meta<Args> = {
  title: "Records/Offers/Line cell",
  args: { onSave: fn(() => Promise.resolve()) },
  parameters: { layout: "padded" },
  render: (args) => (
    <StoryProviders>
      <DiscountCell onSave={args.onSave} />
    </StoryProviders>
  ),
};
export default meta;
type Story = StoryObj<Args>;

export const Saved: Story = {};

/** A discount over 100 % stays in the cell with its reason, and is not saved. */
export const DiscountRefused: Story = {
  play: async ({ args }) => {
    const cell = await screen.findByTestId("line-discount");
    await userEvent.clear(cell);
    await userEvent.type(cell, "150");
    await userEvent.tab();
    await screen.findByText("Discount % must be from 0% to 100%.");
    await expect(cell).toHaveAttribute("aria-invalid", "true");
    await expect(args.onSave).not.toHaveBeenCalled();
  },
};
