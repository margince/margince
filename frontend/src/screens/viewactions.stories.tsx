// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { SavedView } from "./savedviews.queries";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";
import { DeleteViewAction, RenameViewAction } from "./viewactions";

// One saved view's two verbs, each its own dialog: Rename opens on the name it
// has now, and Delete names the view and says no record changes.
const meta: Meta = { title: "Patterns/Filters and views/View actions" };
export default meta;

type Story = StoryObj;

const VIEW: SavedView = {
  id: "v1",
  owner_id: "00000000-0000-4000-8000-000000000001",
  shared_scope: "private",
  resource: "contacts",
  name: "Berlin contacts",
  query: { filter: { and: [{ field: "city", op: "eq", value: "Berlin" }] } },
  version: 3,
};

function routes() {
  installFetchStub({
    "GET /me": meRoute({}),
    "PATCH /views/v1": (body) => jsonResponse({ ...VIEW, ...(body as object) }),
    "DELETE /views/v1": () => new Response(null, { status: 204 }),
  });
}

export const RenameOpen: Story = {
  render: () => {
    routes();
    return (
      <StoryProviders>
        <RenameViewAction view={VIEW} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Rename" }),
    );
  },
};

// The overlay, the dialog's ground and the field on it are three elevations a
// darker palette compresses.
export const RenameOpenDark: Story = {
  ...RenameOpen,
  globals: { theme: "dark" },
};

export const DeleteOpen: Story = {
  render: () => {
    routes();
    return (
      <StoryProviders>
        <DeleteViewAction view={VIEW} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Delete view" }),
    );
  },
};
