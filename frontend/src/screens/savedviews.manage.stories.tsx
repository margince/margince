// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { ManageViewsButton } from "./savedviews.manage";
import { StoryProviders } from "./story-utils";

const meta: Meta = { title: "Patterns/Manage saved views" };
export default meta;

type Story = StoryObj;

type SavedView = components["schemas"]["SavedView"];

const view = (id: string, name: string): SavedView => ({
  id,
  owner_id: "u-1",
  shared_scope: "private",
  resource: "companies",
  name,
  version: 1,
  query: {},
});

/** The reader's own views, each with its rename and delete verbs. */
export const Managing: Story = {
  render: () => (
    <StoryProviders>
      <ManageViewsButton
        resource="companies"
        views={[
          view("v-1", "German customers"),
          view("v-2", "Renewals due this quarter"),
          view(
            "v-3",
            "Manufacturing accounts with an open deal over fifty thousand",
          ),
        ]}
      />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Manage views",
      }),
    );
    await within(document.body).findByRole("dialog");
  },
};
