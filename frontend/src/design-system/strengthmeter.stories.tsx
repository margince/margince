// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { STRENGTH_BANDS, StrengthMeter } from "./strengthmeter";

const meta: Meta<typeof StrengthMeter> = {
  title: "Components/Status indicators/Strength meter",
  component: StrengthMeter,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof StrengthMeter>;

const WORDS = {
  strong: "strong",
  moderate: "moderate",
  weak: "weak",
  none: "no contact",
} as const;

export const EveryBand: Story = {
  render: () => (
    <div style={{ display: "flex", gap: "var(--space-6)" }}>
      {STRENGTH_BANDS.map((band) => (
        <StrengthMeter key={band} band={band} label={WORDS[band]} />
      ))}
    </div>
  ),
};
