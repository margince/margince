// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LiveExplanation } from "./listexplain";
import { listsMe, liveWhy, notOnLiveWhy } from "./lists.fixtures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// A Live List's filter judged for one record, clause by clause, with the
// record's value beside each clause or "hidden" where the reader may not see it.
const meta: Meta = {
  title: "Records/List explanation",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function stub() {
  installFetchStub({
    "GET /me": listsMe(true),
    "GET /filters/vocabulary": () =>
      jsonResponse({ resource: "company", fields: [] }),
  });
}

export const OnTheList: Story = {
  render: () => {
    stub();
    return (
      <StoryProviders>
        <LiveExplanation entityType="company" why={liveWhy} />
      </StoryProviders>
    );
  },
};

export const NotOnTheList: Story = {
  render: () => {
    stub();
    return (
      <StoryProviders>
        <LiveExplanation entityType="company" why={notOnLiveWhy} />
      </StoryProviders>
    );
  },
};
