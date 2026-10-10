// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { AutonomyTiersCard } from "./settings-autonomytiers";
import { StoryProviders } from "./story-utils";

const meta: Meta<typeof AutonomyTiersCard> = {
  title: "Settings/You/Agents/Autonomy tiers",
  component: AutonomyTiersCard,
  render: () => (
    <StoryProviders>
      <AutonomyTiersCard />
    </StoryProviders>
  ),
};
export default meta;
type Story = StoryObj<typeof AutonomyTiersCard>;

export const Tiers: Story = {};

export const TiersDark: Story = { globals: { theme: "dark" } };

// At 390px the dot keeps its word on its line, and Locked wraps as a whole.
export const TiersPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
