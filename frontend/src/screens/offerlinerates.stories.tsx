// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { type LineRates, NewLineRates } from "./offerlinerates";
import { StoryProviders } from "./story-utils";
import "./offers.css";

// The discount and tax controls on an offer's add-line form, in the row the
// form draws them in. Empty means the stored default rather than zero, so an
// empty pair and a filled one are the two states worth seeing.

function Harness({ initial }: Readonly<{ initial: LineRates }>) {
  const [value, setValue] = useState(initial);
  return (
    <div className="offers-new-line">
      <NewLineRates value={value} onChange={setValue} />
    </div>
  );
}

const meta: Meta<typeof Harness> = {
  title: "Records/Offers/Line rates",
  component: Harness,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof Harness>;

export const Defaults: Story = {
  args: { initial: { discount_pct: "", tax_rate: "" } },
};

export const Filled: Story = {
  args: { initial: { discount_pct: "10", tax_rate: "19" } },
};
