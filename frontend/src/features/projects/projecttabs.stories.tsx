// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../testing/story-utils";
import { ProjectTabs } from "./projecttabs";

// The strip on its own, away from the project page. It has one tab that is already
// the page the reader is on, and nothing at its far end.

const meta: Meta<typeof ProjectTabs> = {
  title: "Records/Project 360/Tabs",
  component: ProjectTabs,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ProjectTabs>;

/** The one Overview tab. */
export const Overview: Story = {
  render: () => (
    <StoryProviders>
      <ProjectTabs />
    </StoryProviders>
  ),
};
