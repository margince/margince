// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { SavedView } from "../../lib/savedviews.queries";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
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
    "GET /views": () =>
      jsonResponse({
        data: [VIEW, { ...VIEW, id: "v2", name: "Munich contacts" }],
        page: { next_cursor: null, has_more: false },
      }),
    "PATCH /views/v1": (body) => jsonResponse({ ...VIEW, ...(body as object) }),
    "DELETE /views/v1": () => new Response(null, { status: 204 }),
  });
}

export const RenameOpen: Story = {
  render: () => {
    routes();
    return (
      <StoryProviders>
        <RenameViewAction view={VIEW} resource="contacts" />
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

// A name another view of the same records holds is refused on the field.
export const RenameTakenName: Story = {
  ...RenameOpen,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Rename" }),
    );
    const page = within(canvasElement.ownerDocument.body);
    const name = await page.findByRole("textbox", { name: "Name" });
    await userEvent.clear(name);
    await userEvent.type(name, "Munich contacts");
    await userEvent.click(
      within(await page.findByRole("dialog")).getByRole("button", {
        name: "Rename",
      }),
    );
    await page.findByText(
      "A view with this name already exists. Choose another name.",
    );
  },
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
