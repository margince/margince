// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../testing/story-utils";
import { AboutPage } from "./aboutpage";

const meta = {
  title: "Settings/System/About Margince/Page",
  component: AboutPage,
  parameters: { layout: "padded" },
  render: () => (
    <StoryProviders>
      <div className="settings-stack">
        <AboutPage />
      </div>
    </StoryProviders>
  ),
} satisfies Meta<typeof AboutPage>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Dark: Story = { globals: { theme: "dark" } };

export const Phone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
