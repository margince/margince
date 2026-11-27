// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import type { SavedView } from "../../lib/savedviews.queries";
import {
  listsMe,
  liveList,
  READER_ID,
  shortlist,
  TEAM_ID,
  teamsPage,
} from "../../testing/lists.fixtures";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { LibraryGroupPanel } from "./filterslibrary.group";
import type { LibraryReads } from "./filterslibrary.reads";
import { type LibraryItem, libraryItems } from "./library";

// One group of the library: its rows run edge to edge in the panel. What its
// reads have not answered yet stands in a padded band of its own.
const meta: Meta = {
  title: "Patterns/Filters and views/Library group",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const BERLIN: SavedView = {
  id: "v1",
  owner_id: READER_ID,
  resource: "contacts",
  name: "Berlin contacts",
  query: { filter: { and: [{ field: "city", op: "eq", value: "Berlin" }] } },
  version: 1,
};

const ROWS = libraryItems(
  [
    BERLIN,
    {
      ...BERLIN,
      id: "v-name",
      name: "By name",
      query: { list: { sort: "full_name" } },
    },
  ],
  [
    {
      ...shortlist,
      id: "L-dinner",
      name: "Dinner guests",
      owner_id: READER_ID,
    },
  ],
);

const ANSWERED: LibraryReads = {
  items: ROWS,
  settled: true,
  failed: false,
  viewsPending: false,
  viewsFailed: false,
  listsPending: false,
  listsFailed: false,
  listsHeld: false,
  searchPending: false,
  unsettled: () => false,
  truncated: false,
  countable: true,
  limit: "200",
  retryViews: fn(),
  retryLists: fn(),
};

const captionOf = (item: LibraryItem) => {
  switch (item.kind) {
    case "list":
      return item.list.purpose ?? "";
    case "listView":
      return "Saved on the Contacts list";
    case "view":
      return "City is Berlin";
  }
};

function group(
  rows: readonly LibraryItem[],
  reads: LibraryReads,
  which: "mine" | "shared" = "mine",
  folded = false,
) {
  installFetchStub({
    "GET /me": listsMe(true, [TEAM_ID]),
    "GET /teams": () => jsonResponse(teamsPage),
  });
  return (
    <StoryProviders>
      <LibraryGroupPanel
        group={which}
        rows={rows}
        reads={reads}
        narrowed={false}
        captionOf={captionOf}
        folded={folded}
        returnFocusTo={() => null}
      />
    </StoryProviders>
  );
}

export const Rows: Story = {
  render: () => group(ROWS, ANSWERED),
};

export const RowsDark: Story = {
  ...Rows,
  globals: { theme: "dark" },
};

// Folded, the facts move under the name and the rows still meet the edges.
export const RowsPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => group(ROWS, ANSWERED, "mine", true),
};

export const Loading: Story = {
  render: () =>
    group([], {
      ...ANSWERED,
      settled: false,
      viewsPending: true,
      listsPending: true,
    }),
};

// No one has shared a list yet: the line keeps the panel's padding.
export const Empty: Story = {
  render: () => group([], ANSWERED, "shared"),
};

// The lists read failed: its line and Retry stand above the views it did read.
export const ReadFailed: Story = {
  render: () =>
    group(
      ROWS.filter((item) => item.kind !== "list"),
      {
        ...ANSWERED,
        failed: true,
        listsFailed: true,
      },
    ),
  play: async ({ canvasElement }) => {
    const failed = await within(canvasElement).findByRole("alert");
    await expect(
      within(failed).getByRole("button", { name: "Retry" }),
    ).toBeVisible();
  },
};

export const LiveListRow: Story = {
  render: () => group(libraryItems([], [liveList]), ANSWERED, "shared"),
};
