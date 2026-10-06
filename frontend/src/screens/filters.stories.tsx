// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { FiltersScreen } from "./filters";
import { listsMe } from "./lists.fixtures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The dispatcher: what an address shows before the session answers, and what
// it opens once it has. The pages it opens have their own stories (Library,
// New filter); these are the states that belong to the dispatch itself.
const meta: Meta<typeof FiltersScreen> = {
  title: "Patterns/Filters and views/Filter page",
  component: FiltersScreen,
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

type Story = StoryObj<typeof FiltersScreen>;

function routes(mePending = false): void {
  installFetchStub({
    "GET /me": mePending
      ? () => new Promise<Response>(() => {})
      : listsMe(false),
    "GET /views": () =>
      jsonResponse({
        data: [
          {
            id: "v-1",
            owner_id: "u-1",
            resource: "contacts",
            name: "Berlin contacts",
            shared_scope: "private",
            query: {
              filter: { and: [{ field: "city", op: "eq", value: "Berlin" }] },
            },
            version: 1,
          },
        ],
        page: { next_cursor: null, has_more: false },
      }),
  });
}

// The library waits unheaded: the shell names it, and nothing below the
// heading can be drawn until the session says whether lists are on.
export const MePendingOnLibrary: Story = {
  render: () => {
    routes(true);
    return <FiltersScreen />;
  },
};

// A focused page names itself, so it wears its fallback name while it waits.
export const MePendingOnFocusedPage: Story = {
  render: () => {
    routes(true);
    return <FiltersScreen id="contacts" />;
  },
};

// Bare `#/filters` lands on the library, never on a builder.
export const LibraryLanding: Story = {
  render: () => {
    routes();
    return <FiltersScreen />;
  },
};
