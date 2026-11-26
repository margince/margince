// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../i18n";
import { SegmentBar, SegmentSwatch } from "./segmentbar";

const meta: Meta = {
  title: "Components/Text and data display/Segment bar",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <div style={{ maxWidth: 480, display: "grid", gap: "var(--space-5)" }}>
          <Story />
        </div>
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj;

// Disjoint parts of one total with a target marked across them. The call lands
// inside the parts in the first frame and past them in the second. The second
// leaves recessed track between the last part and the mark.
export const SegmentBars: Story = {
  render: () => (
    <>
      <SegmentBar
        label="How the period is made up"
        parts={[
          { key: "won", label: "Already won", value: 92_000, amount: "€92K" },
          {
            key: "evidence",
            label: "Evidence",
            value: 148_000,
            amount: "€148K",
          },
          {
            key: "best",
            label: "Best case beyond evidence",
            value: 112_000,
            amount: "€112K",
          },
        ]}
        marker={{
          key: "call",
          label: "Current call",
          value: 200_000,
          amount: "€200K",
        }}
      />
      <SegmentBar
        label="How the period is made up"
        parts={[
          { key: "won", label: "Already won", value: 40_000, amount: "€40K" },
          { key: "evidence", label: "Evidence", value: 60_000, amount: "€60K" },
        ]}
        marker={{
          key: "call",
          label: "Current call",
          value: 180_000,
          amount: "€180K",
        }}
      />
      <SegmentBar
        label="Closed revenue against the goal"
        parts={[
          {
            key: "product",
            label: "Product sales",
            value: 91_000,
            amount: "€91K",
          },
          {
            key: "service",
            label: "Services / projects",
            value: 88_000,
            amount: "€88K",
          },
          {
            key: "unresolved",
            label: "Unclassified",
            value: 4_000,
            amount: "€4K",
          },
        ]}
        ahead={{
          key: "planned",
          label: "Planned, not counted",
          value: 85_000,
          amount: "€85K",
        }}
        marker={{ key: "goal", label: "Goal", value: 250_000, amount: "€250K" }}
      />
    </>
  ),
};

// Readings beside the bar name each part under its swatch, so the bar keeps
// its legend for a screen reader alone.
export const SegmentBarWithSwatches: Story = {
  render: () => (
    <>
      <SegmentBar
        legend="hidden"
        label="What the sales won are made of"
        parts={[
          { key: "product", label: "Products", value: 38_000, amount: "€38K" },
          { key: "service", label: "Services", value: 22_000, amount: "€22K" },
        ]}
        ahead={{
          key: "open",
          label: "Open pipeline",
          value: 30_000,
          amount: "€30K",
        }}
        marker={{ key: "goal", label: "Goal", value: 100_000, amount: "€100K" }}
      />
      <p>
        <SegmentSwatch step={0} /> Products · <SegmentSwatch step={1} />{" "}
        Services · <SegmentSwatch step="ahead" /> Open pipeline
      </p>
    </>
  ),
};
