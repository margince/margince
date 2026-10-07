// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { type AttentionItem, AttentionPanel } from "./analytics.attention";
import { StoryProviders } from "./story-utils";

const meta: Meta = { title: "Records/Reports/Needs your attention" };
export default meta;

type Story = StoryObj;

const checks: AttentionItem = {
  key: "checks",
  title: "2 forecast checks to answer",
  detail: "Answer them before you update the forecast.",
  action: "Review in Forecast",
  section: "forecast",
};

const unpriced: AttentionItem = {
  key: "unpriced",
  title: "10 of 12 open deals are priced",
  detail: "A deal without a price adds nothing to the forecast.",
  action: "Open Forecast",
  section: "forecast",
};

const coverage: AttentionItem = {
  key: "coverage",
  title: "1 data source was not fully checked",
  detail:
    "For sources that could not be checked, ask the connection owner or your administrator to restore access.",
  action: "View data coverage",
  section: "coverage",
};

// Every source has something to say: the order is checks, prices, coverage.
export const AllItems: Story = {
  render: () => (
    <StoryProviders>
      <AttentionPanel items={[checks, unpriced, coverage]} />
    </StoryProviders>
  ),
};

export const OneItem: Story = {
  render: () => (
    <StoryProviders>
      <AttentionPanel items={[checks]} />
    </StoryProviders>
  ),
};

// The action drops under the words rather than squeezing them.
export const Phone: Story = {
  ...AllItems,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
