// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ShortlistVerb } from "./bulkshortlist";
import { listsMe, shortlist } from "./lists.fixtures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// "Add to Shortlist" in the bulk bar: the Shortlists this reader looks after,
// less the one the rows are already shown on.
const meta: Meta = {
  title: "Records/Bulk change/Add to Shortlist",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const other = {
  ...shortlist,
  id: "01a0f000-0000-7000-8000-000000000099",
  name: "Renewal calls",
};

export const Picker: Story = {
  render: () => {
    installFetchStub({
      "GET /me": listsMe(true),
      "GET /lists": () =>
        jsonResponse({ data: [shortlist, other], page: { has_more: false } }),
    });
    return (
      <StoryProviders>
        <ShortlistVerb
          recordType="company"
          disabled={false}
          exclude={shortlist.id}
          onPick={() => {}}
        />
      </StoryProviders>
    );
  },
};
