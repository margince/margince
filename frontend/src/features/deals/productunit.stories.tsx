// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { StoryProviders } from "../../testing/story-utils";
import { type UnitChoice, UnitPicker, unitChoiceOf } from "./productunit";
import "./offers.css";

// The unit control of an offer line. A key shows as a word in the reader's
// language; "Other" opens a box for a unit the list lacks.

function Harness({
  stored,
  blankLabel,
}: Readonly<{ stored: string; blankLabel?: string }>) {
  const [value, setValue] = useState<UnitChoice>(unitChoiceOf(stored));
  return (
    <div className="offers-new-line">
      <UnitPicker
        testId="story-unit"
        blankLabel={blankLabel}
        value={value}
        onChange={setValue}
      />
    </div>
  );
}

const meta: Meta<typeof Harness> = {
  title: "Records/Offers/Line unit",
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

export const Key: Story = { args: { stored: "day" } };

export const TypedUnit: Story = { args: { stored: "Lizenz" } };

export const FromTheProduct: Story = {
  args: { stored: "", blankLabel: "From the product" },
};
