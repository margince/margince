// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useMemberColumns } from "./listmembercolumns";
import { MemberRows } from "./listmembers";
import {
  chosenListing,
  LIVE_ID,
  listingAnswer,
  listsMe,
  liveList,
  liveListing,
  members,
  SHORTLIST_ID,
  shortlist,
} from "./lists.fixtures";
import type { List } from "./lists.queries";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// A list's members with what a reader can do to them: tick rows for the bulk
// bar, select every member, or export the list. A Shortlist's columns say who
// chose each member; a Live List's show the fields its filter names.
const meta: Meta = {
  title: "Records/List members",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const NOBODY_NEW: ReadonlySet<string> = new Set();

function Members({ list }: Readonly<{ list: List }>) {
  const columns = useMemberColumns(list, NOBODY_NEW);
  return (
    <MemberRows
      list={list}
      source="company"
      onOpen={() => {}}
      columns={columns}
    />
  );
}

export const ShortlistMembers: Story = {
  render: () => {
    installFetchStub({
      "GET /me": listsMe(true),
      "GET /companies": () =>
        jsonResponse({ data: members, page: { has_more: false } }),
      [`GET /lists/${SHORTLIST_ID}/members`]: listingAnswer(chosenListing),
    });
    return (
      <StoryProviders>
        <Members list={shortlist} />
      </StoryProviders>
    );
  },
};

export const LiveListMembers: Story = {
  render: () => {
    installFetchStub({
      "GET /me": listsMe(true),
      "GET /companies": () =>
        jsonResponse({ data: members, page: { has_more: false } }),
      [`GET /lists/${LIVE_ID}/members`]: listingAnswer(liveListing),
      "GET /filters/vocabulary": () =>
        jsonResponse({ resource: "company", fields: [] }),
    });
    return (
      <StoryProviders>
        <Members list={liveList} />
      </StoryProviders>
    );
  },
};
