// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { ManageViewsButton } from "./savedviews.manage";
import type { SavedView } from "./savedviews.queries";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";

// The dialog a list's own view rail manages its views in: every view of the
// list, each row opening in place into a rename or a delete, so a second
// dialog never stacks on the first.
const meta: Meta = { title: "Patterns/Manage views" };
export default meta;

type Story = StoryObj;

const view = (id: string, name: string): SavedView => ({
  id,
  owner_id: "00000000-0000-4000-8000-000000000001",
  shared_scope: "private",
  resource: "companies",
  name,
  query: { list: { q: "", sort: "", includeArchived: false, filters: {} } },
  version: 1,
});

export const ManageViewsOpen: Story = {
  render: () => {
    installFetchStub({ "GET /me": meRoute({}) });
    return (
      <StoryProviders>
        <ManageViewsButton
          views={[
            view("v1", "German customers"),
            view("v2", "Churned in 2026"),
            view(
              "v3",
              "Manufacturing accounts with an open deal over fifty thousand",
            ),
          ]}
        />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Manage views" }),
    );
    await within(document.body).findByRole("dialog");
  },
};

// Its rows, ghost buttons and the dialog's ground are all derived tokens, so
// the list can read in light and be wrong in dark.
export const ManageViewsOpenDark: Story = {
  ...ManageViewsOpen,
  globals: { theme: "dark" },
};
