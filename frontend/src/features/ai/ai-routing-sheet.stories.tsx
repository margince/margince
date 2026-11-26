// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../testing/story-utils";
import { SheetFooter } from "./ai-routing-sheet";

const meta = {
  title: "Settings/AI/AI models/Model tiers footer",
  component: SheetFooter,
  render: (args) => (
    <StoryProviders>
      <SheetFooter {...args} />
    </StoryProviders>
  ),
  args: {
    catalogue: [],
    profile: "Cloud (cloud providers with the installation’s own API keys)",
  },
} satisfies Meta<typeof SheetFooter>;
export default meta;
type Story = StoryObj<typeof meta>;

export const NamedProfile: Story = {};
export const NamedProfileDark: Story = { globals: { theme: "dark" } };
export const NamedProfilePhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
