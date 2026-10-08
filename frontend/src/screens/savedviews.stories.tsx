// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { type GrantSpec, meFixture } from "../app/mefixture";
import type { ListQuery } from "./listquery";
import { SaveViewAction } from "./savedviews";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// A saved view is per-user list state, by name: saved beside a list's own
// tools through the catalog's one naming dialog, and managed from the same
// place.
//
// The offering rule is the other half. Save renders NOTHING until there is
// something worth saving — an unnarrowed list gets no button — so the story
// that shows the withheld state carries as much as the one that shows it.
const meta: Meta = {
  title: "Patterns/Saved views",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;

const VIEWS = {
  data: [
    {
      id: "v-1",
      owner_id: "u-1",
      resource: "contacts",
      name: "Gold tier in Berlin",
      query: {
        filter: { and: [{ field: "city", op: "eq", value: "Berlin" }] },
      },
      version: 1,
    },
    {
      // `like` is not an operator this engine has, so this build cannot read
      // the stored tree; the view is still the reader's to rename or delete.
      id: "v-2",
      owner_id: "u-1",
      resource: "contacts",
      name: "Saved by an older build",
      query: { filter: { and: [{ field: "city", op: "like", value: "Ber" }] } },
      version: 1,
    },
  ],
  page: { next_cursor: null, has_more: false },
};

// A saved view is per-USER, so every surface here sits behind a session — the
// rail reads it to decide whose views these are. Routed explicitly rather than
// left to the stub's fallback: an unrouted /me answers a list shape, which
// reads as a malformed session, fails every grant closed, and renders a
// refusal none of these stories is named for.
const SESSION: GrantSpec = {
  saved_view: ["read", "create"],
  contact: ["read"],
};

function routes(extra: Parameters<typeof installFetchStub>[0] = {}): void {
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture({ allow: SESSION })),
    "GET /views": () => jsonResponse(VIEWS),
    ...extra,
  });
}

const NARROWED: ListQuery = {
  q: "ann",
  sort: "-created_at",
  includeArchived: false,
  filters: { owner: "me" },
  perPage: 25,
};

const UNNARROWED: ListQuery = {
  q: "",
  sort: "",
  includeArchived: false,
  filters: {},
  perPage: 25,
};

type Story = StoryObj;

export const NamingAView: Story = {
  // The naming dialog, opened.
  render: () => {
    routes();
    return <SaveViewAction resource="contacts" query={NARROWED} />;
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Save view" }));
  },
};

export const NothingWorthSaving: Story = {
  // An unnarrowed list offers no save: the view would do what the All tab
  // already does, and a rail of those is how a useful feature becomes clutter.
  // Manage views stays, because the reader's existing views are still theirs
  // to rename or delete.
  render: () => {
    routes();
    return <SaveViewAction resource="contacts" query={UNNARROWED} />;
  },
};

export const ManagingViews: Story = {
  // Every saved view of the resource, each with Rename and Delete. The first
  // row is opened into its rename, the one question a row asks in place.
  render: () => {
    routes({
      "PATCH /views/v-1": () => jsonResponse(VIEWS.data[0]),
      "DELETE /views/v-1": () => jsonResponse(VIEWS.data[0]),
    });
    return <SaveViewAction resource="contacts" query={UNNARROWED} />;
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Manage views" }),
    );
    const page = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await page.findByRole("button", { name: "Rename Gold tier in Berlin" }),
    );
  },
};

export const TheRailFailedToLoad: Story = {
  // The one place that says the saved-view rail did not load, and it says WHICH
  // surface: the notice lands beside a list's Columns and Compact buttons, where
  // an unnamed "this section did not load" could be any of the three.
  render: () => {
    routes({
      "GET /views": () => jsonResponse({ title: "Server error" }, 500),
    });
    return <SaveViewAction resource="contacts" query={NARROWED} />;
  },
};
