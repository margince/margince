// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useMemberColumns } from "./listmembercolumns";
import { MembersPanel } from "./listmembers";
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

// A list's members as one Panel: Select all in its head, and the table as its
// own content, ticked rows feeding the bulk bar. A Shortlist's columns say who
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
    <MembersPanel
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

// The table sheds its own box on the Panel ground, so in dark no elevated
// rectangle or shadow is left behind it, and the Display band reads recessed.
export const LiveListMembersDark: Story = {
  ...LiveListMembers,
  globals: { theme: "dark" },
};

// At phone width only the title gives way in the head, and the rows fold into
// the table's own phone layout. `uat-phone` drives the capture gate to 390px.
export const ShortlistMembersPhone: Story = {
  ...ShortlistMembers,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
