// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { ListLibrary } from "./listlibrary";
import {
  listsMe,
  liveList,
  shortlist,
  TEAM_ID,
  teamsPage,
} from "./lists.fixtures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// Shared views: one row per list, with what it is for, how many members the
// reader can see, who looks after it and who can find it.
const meta: Meta = { title: "Patterns/List library" };
export default meta;

type Story = StoryObj;

// Three audiences: one named team, the reader's own teams, and everyone.
export const ThreeLists: Story = {
  render: () => {
    installFetchStub({
      "GET /me": listsMe(true, [TEAM_ID]),
      "GET /teams": () => jsonResponse(teamsPage),
      "GET /lists": () =>
        jsonResponse({
          data: [
            {
              ...liveList,
              id: "named-team",
              name: "German accounts",
              team_id: TEAM_ID,
            },
            liveList,
            shortlist,
          ],
          page: { has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <ListLibrary />
      </StoryProviders>
    );
  },
};

export const NoListsYet: Story = {
  render: () => {
    installFetchStub({
      "GET /me": listsMe(true),
      "GET /lists": () => jsonResponse({ data: [], page: { has_more: false } }),
    });
    return (
      <StoryProviders>
        <ListLibrary />
      </StoryProviders>
    );
  },
};

/** New Shortlist pressed: its name, what it is for, its record type and audience. */
export const StartingAShortlist: Story = {
  render: () => {
    installFetchStub({
      "GET /me": listsMe(true, [TEAM_ID]),
      "GET /teams": () => jsonResponse(teamsPage),
      "GET /lists": () => jsonResponse({ data: [], page: { has_more: false } }),
    });
    return (
      <StoryProviders>
        <ListLibrary />
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
