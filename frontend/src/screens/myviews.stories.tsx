// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { listsMe, shortlist } from "./lists.fixtures";
import { MyViews } from "./myviews";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// My views: the reader's private lists, then their saved filters, one group per
// record type. A list row opens its page, a view row opens the builder with
// that view loaded; an empty group says how to add one.
const meta: Meta = { title: "Patterns/My views" };
export default meta;

type Story = StoryObj;

const empty = { data: [], page: { has_more: false } };

export const SomeViews: Story = {
  render: () => {
    installFetchStub({
      "GET /me": listsMe(true),
      "GET /lists": () =>
        jsonResponse({
          data: [{ ...shortlist, name: "Dinner guests", sharing: "private" }],
          page: { has_more: false },
        }),
      "GET /views": () =>
        jsonResponse({
          data: [
            {
              id: "v1",
              name: "Berlin decision makers",
              resource: "contacts",
              query: { filter: { field: "city", op: "eq", value: "Berlin" } },
              version: 1,
            },
          ],
          page: { has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <MyViews />
      </StoryProviders>
    );
  },
};

export const NoViewsYet: Story = {
  render: () => {
    installFetchStub({
      "GET /me": listsMe(true),
      "GET /views": () => jsonResponse(empty),
    });
    return (
      <StoryProviders>
        <MyViews />
      </StoryProviders>
    );
  },
};
