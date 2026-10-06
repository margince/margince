// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { type LibraryItem, libraryItems } from "./library";
import { LibraryTable, NewShortlistAction } from "./listlibrary";
import {
  listsMe,
  liveList,
  OTHER_OWNER_ID,
  shortlist,
  TEAM_ID,
  teamsPage,
} from "./lists.fixtures";
import { DEFAULT_AUDIENCE } from "./listsharing";
import type { SavedView } from "./savedviews.queries";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The one table every library group draws: a saved view and a list read alike,
// the name a link stretched over its row, and only a list counted. A kind is
// neutral except a Live List's, which wears info's tint and the live dot.
// Shared adds who can find each row; a phone folds the facts under the name.
const meta: Meta = { title: "Patterns/Filters and views/Library table" };
export default meta;

type Story = StoryObj;

const BERLIN: SavedView = {
  id: "v1",
  owner_id: "00000000-0000-4000-8000-000000000001",
  shared_scope: "private",
  resource: "contacts",
  name: "Berlin contacts",
  query: { filter: { and: [{ field: "city", op: "eq", value: "Berlin" }] } },
  version: 1,
};

const mine = libraryItems(
  [BERLIN],
  [{ ...shortlist, id: "L-dinner", name: "Dinner guests", sharing: "private" }],
);
const shared = libraryItems(
  [],
  [
    { ...liveList, id: "L-de", name: "German accounts", team_id: TEAM_ID },
    liveList,
    { ...liveList, id: "L-theirs", name: "Theirs", owner_id: OTHER_OWNER_ID },
    shortlist,
  ],
);

/** The caption a row carries: a list's purpose, a view's filter as words. */
const captionOf = (item: LibraryItem) =>
  item.kind === "list" ? (item.list.purpose ?? "") : "City is Berlin";

function table(
  items: readonly LibraryItem[],
  group: "mine" | "shared",
  folded = false,
) {
  installFetchStub({
    "GET /me": listsMe(true, [TEAM_ID]),
    "GET /teams": () => jsonResponse(teamsPage),
  });
  return (
    <StoryProviders>
      <LibraryTable
        label="Library"
        items={items}
        group={group}
        captionOf={captionOf}
        folded={folded}
      />
    </StoryProviders>
  );
}

export const OnlyMeRows: Story = {
  render: () => table(mine, "mine"),
};

export const SharedRowsWithWho: Story = {
  render: () => table(shared, "shared"),
};

// The Live List badge's tint and dot are colour-mixed from the theme's tokens,
// so they are checked in the dark theme too.
export const SharedRowsDark: Story = {
  globals: { theme: "dark" },
  render: () => table(shared, "shared"),
};

// Folded, the kind, the count and who can find it move under the name, and
// ⋯ stays at the row's end.
export const FoldedAtPhoneWidth: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => table(shared, "shared", true),
};

export const ArchivedRow: Story = {
  render: () =>
    table(
      libraryItems(
        [],
        [
          {
            ...shortlist,
            name: "Spring launch invitees",
            health: "ok",
            archived_at: "2026-06-01T00:00:00Z",
          },
        ],
      ),
      "shared",
    ),
};

/** New Shortlist pressed: its name, what it is for, its record type and audience. */
export const StartingAShortlist: Story = {
  render: () => {
    installFetchStub({
      "GET /me": listsMe(true, [TEAM_ID]),
      "GET /teams": () => jsonResponse(teamsPage),
    });
    return (
      <StoryProviders>
        <NewShortlistAction defaultAudience={DEFAULT_AUDIENCE} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "New Shortlist",
      }),
    );
    await within(document.body).findByRole("dialog");
  },
};
