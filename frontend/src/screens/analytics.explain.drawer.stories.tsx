// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import { ExplainDrawer } from "./analytics.explain.drawer";
import { StoryProviders } from "./story-utils";

// The trigger a figure carries and the drawer it opens, with a stand-in body:
// the report cards and the question builder each hand it their own read.
const meta: Meta = { title: "Records/Reports/Explain drawer" };
export default meta;

type Story = StoryObj;

function drawerStory() {
  return (
    <StoryProviders>
      <ExplainDrawer
        figure="Qualified, EUR"
        body={() => <p>Records behind this figure.</p>}
      >
        Qualified
      </ExplainDrawer>
    </StoryProviders>
  );
}

export const Closed: Story = { render: drawerStory };

export const Open: Story = {
  render: drawerStory,
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Explain Qualified, EUR",
      }),
    );
    await screen.findByRole("dialog");
  },
};

export const OpenDark: Story = { ...Open, globals: { theme: "dark" } };
