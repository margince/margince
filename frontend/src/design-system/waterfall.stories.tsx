// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../i18n";
import { Waterfall } from "./waterfall";

// A total, what moved it, and the total it became.
const meta: Meta = {
  title: "Design System/Waterfall",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <div style={{ maxWidth: 560 }}>
          <Story />
        </div>
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj;

const WARNING = "These steps do not add up to the closing total.";

export const Reconciled: Story = {
  render: () => (
    <Waterfall
      label="Movement this week"
      reconciliationWarning={WARNING}
      opening={{ label: "Last Monday", value: 1_200_000, amount: "€1.2M" }}
      closing={{ label: "Today", value: 1_355_000, amount: "€1.355M" }}
      steps={[
        { key: "new", label: "New", value: 240_000, amount: "€240k" },
        {
          key: "pushed_out",
          label: "Pushed out",
          value: -85_000,
          amount: "-€85k",
        },
        { key: "amount", label: "Amount", value: 40_000, amount: "€40k" },
        { key: "lost", label: "Lost", value: -40_000, amount: "-€40k" },
      ]}
    />
  ),
};

// The case the component exists to catch. Drawn rather than hidden, because a
// picture that quietly does not add up is worse than one that says so.
export const DoesNotReconcile: Story = {
  render: () => (
    <Waterfall
      label="Movement this week"
      reconciliationWarning={WARNING}
      opening={{ label: "Last Monday", value: 1_200_000, amount: "€1.2M" }}
      closing={{ label: "Today", value: 1_900_000, amount: "€1.9M" }}
      steps={[{ key: "new", label: "New", value: 100_000, amount: "€100k" }]}
    />
  ),
};

// A quiet week: two equal anchors and nothing between them. It reconciles.
export const NothingMoved: Story = {
  render: () => (
    <Waterfall
      label="Movement this week"
      reconciliationWarning={WARNING}
      opening={{ label: "Last Monday", value: 1_200_000, amount: "€1.2M" }}
      closing={{ label: "Today", value: 1_200_000, amount: "€1.2M" }}
      steps={[]}
    />
  ),
};

// Every bucket at once, which is what a real quarter's movement looks like and
// the case where the column widths and the wrapping labels have to hold.
export const EveryBucket: Story = {
  render: () => (
    <Waterfall
      label="Movement this quarter"
      reconciliationWarning={WARNING}
      opening={{ label: "Quarter start", value: 900_000, amount: "€900k" }}
      closing={{ label: "Today", value: 1_015_000, amount: "€1.015M" }}
      steps={[
        { key: "new", label: "New", value: 300_000, amount: "€300k" },
        { key: "pulled_in", label: "Pulled in", value: 60_000, amount: "€60k" },
        {
          key: "pushed_out",
          label: "Pushed out",
          value: -120_000,
          amount: "-€120k",
        },
        { key: "amount", label: "Amount", value: 25_000, amount: "€25k" },
        { key: "category", label: "Category", value: -15_000, amount: "-€15k" },
        {
          key: "stage_weight",
          label: "Stage weight",
          value: 10_000,
          amount: "€10k",
        },
        { key: "won", label: "Won", value: -80_000, amount: "-€80k" },
        { key: "lost", label: "Lost", value: -45_000, amount: "-€45k" },
        {
          key: "archived",
          label: "Reopened or archived",
          value: -20_000,
          amount: "-€20k",
        },
      ]}
    />
  ),
};
