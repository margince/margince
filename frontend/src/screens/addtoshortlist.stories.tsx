// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { AddToShortlistAction } from "./addtoshortlist";
import { listsMe, MEMBER_ID, shortlist } from "./lists.fixtures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The record page's "Add to Shortlist": the verb, and behind it the choice of
// Shortlist and the note on why. Absent while lists are switched off.
const meta: Meta = { title: "Patterns/Add to Shortlist" };
export default meta;

type Story = StoryObj;

export const Offered: Story = {
  render: () => {
    installFetchStub({
      "GET /me": listsMe(true),
      "GET /lists": () =>
        jsonResponse({ data: [shortlist], page: { has_more: false } }),
    });
    return (
      <StoryProviders>
        <AddToShortlistAction entityType="company" entityId={MEMBER_ID} />
      </StoryProviders>
    );
  },
};
