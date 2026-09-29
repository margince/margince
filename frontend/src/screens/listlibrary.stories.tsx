// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ListLibrary } from "./listlibrary";
import { listsMe, liveList, shortlist } from "./lists.fixtures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The team's lists: one row per list, with what it is for, how many members
// the reader can see and who looks after it.
const meta: Meta = { title: "Patterns/List library" };
export default meta;

type Story = StoryObj;

export const TwoLists: Story = {
  render: () => {
    installFetchStub({
      "GET /me": listsMe(true),
      "GET /lists": () =>
        jsonResponse({
          data: [liveList, shortlist],
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
