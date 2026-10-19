// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Rings } from "./rings";

const meta: Meta<typeof Rings> = {
  title: "Components/Text and data display/Rings",
  component: Rings,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof Rings>;

// A company's three ratings, each a third of the ring per step up its scale.
export const ThreeReadings: Story = {
  args: {
    label: "Relationship good, commercial at risk, payment strong",
    rings: [
      { key: "relationship", value: 2, max: 3, tone: "calm" },
      { key: "commercial", value: 1, max: 3, tone: "danger" },
      { key: "payment", value: 3, max: 3, tone: "calm" },
    ],
  },
};

// A reading not taken draws its track and no arc, and no tone.
export const NotAssessed: Story = {
  args: {
    label: "Relationship good, commercial at risk, payment not assessed",
    rings: [
      { key: "relationship", value: 2, max: 3, tone: "calm" },
      { key: "commercial", value: 1, max: 3, tone: "warning" },
      { key: "payment", value: 0, max: 3 },
    ],
  },
};
