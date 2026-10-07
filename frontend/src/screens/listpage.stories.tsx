// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ListScreen } from "./listpage";
import {
  chosenListing,
  history,
  LIVE_ID,
  listingAnswer,
  listsMe,
  liveHistory,
  liveList,
  liveListing,
  members,
  SHORTLIST_ID,
  shortlist,
  visitAnswer,
} from "./lists.fixtures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// One list opened: its purpose, its count, who looks after it, its members and
// what changed. The list page heads itself, so the story is fullscreen.
const meta: Meta = {
  title: "Records/List page",
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj;

const page = { has_more: false };

// A Live List a reader can change: members from its filter, each beside the
// values of the fields the filter names, one of them hidden from this reader.
export const LiveList: Story = {
  render: () => {
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${LIVE_ID}`]: () => jsonResponse(liveList),
      [`POST /lists/${LIVE_ID}/visit`]: visitAnswer(LIVE_ID),
      [`GET /lists/${LIVE_ID}/history`]: () =>
        jsonResponse({ data: liveHistory, page }),
      "GET /companies": () => jsonResponse({ data: members, page }),
      [`GET /lists/${LIVE_ID}/members`]: listingAnswer(liveListing),
    });
    return (
      <StoryProviders>
        <ListScreen listID={LIVE_ID} />
      </StoryProviders>
    );
  },
};

// A Shortlist whose steward is gone: the notice asks somebody to take it over,
// and each member says who chose it, when and why.
export const ShortlistNobodyLooksAfter: Story = {
  render: () => {
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${SHORTLIST_ID}`]: () => jsonResponse(shortlist),
      [`POST /lists/${SHORTLIST_ID}/visit`]: visitAnswer(SHORTLIST_ID),
      [`GET /lists/${SHORTLIST_ID}/history`]: () =>
        jsonResponse({ data: history, page }),
      "GET /companies": () => jsonResponse({ data: members, page }),
      [`GET /lists/${SHORTLIST_ID}/members`]: listingAnswer(chosenListing),
    });
    return (
      <StoryProviders>
        <ListScreen listID={SHORTLIST_ID} />
      </StoryProviders>
    );
  },
};

// An archived list reads as read-only, with the one verb that brings it back.
export const Archived: Story = {
  render: () => {
    installFetchStub({
      "GET /me": listsMe(true),
      [`GET /lists/${LIVE_ID}`]: () =>
        jsonResponse({ ...liveList, archived_at: "2026-09-25T10:00:00Z" }),
      [`POST /lists/${LIVE_ID}/visit`]: visitAnswer(LIVE_ID),
      [`GET /lists/${LIVE_ID}/history`]: () => jsonResponse({ data: [], page }),
      "GET /companies": () => jsonResponse({ data: [], page }),
    });
    return (
      <StoryProviders>
        <ListScreen listID={LIVE_ID} />
      </StoryProviders>
    );
  },
};

// With lists switched off the page says so, and reads nothing.
export const ListsSwitchedOff: Story = {
  render: () => {
    installFetchStub({ "GET /me": listsMe(false) });
    return (
      <StoryProviders>
        <ListScreen listID={LIVE_ID} />
      </StoryProviders>
    );
  },
};
