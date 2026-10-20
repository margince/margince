// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Rings, RingsLegend } from "./rings";

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

// The figure with its legend, as a contact's 360 draws it.
export const WithLegend: Story = {
  render: () => (
    <RingsLegend
      label="Conversation: Dana sent 9 of 20 messages, Reach: 1 of 3 colleagues in touch, Commitments: 1 of 2 commitments kept"
      rings={[
        {
          key: "conversation",
          value: 9,
          max: 20,
          tone: "calm",
          label: "Conversation",
          caption: "Dana sent 9 of 20 messages",
        },
        {
          key: "reach",
          value: 1,
          max: 3,
          tone: "warning",
          label: "Reach",
          caption: "1 of 3 colleagues in touch",
        },
        {
          key: "commitments",
          value: 1,
          max: 2,
          tone: "warning",
          label: "Commitments",
          caption: "1 of 2 commitments kept",
        },
      ]}
    />
  ),
};
