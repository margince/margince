// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { TagVerbs } from "./bulktag";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// "Add tag" and "Remove tag" in the bulk bar: a tag picker and the two verbs,
// which wait for a tag to be picked.
const meta: Meta = {
  title: "Records/Bulk change/Tag",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function Verbs() {
  installFetchStub({
    "GET /me": meRoute({}),
    "GET /tags": () =>
      jsonResponse({
        data: [
          { id: "t-key", name: "Key account", version: 1 },
          { id: "t-launch", name: "Launch reference", version: 1 },
        ],
        page: { has_more: false },
      }),
  });
  return (
    <StoryProviders>
      <TagVerbs disabled={false} onPick={() => {}} />
    </StoryProviders>
  );
}

export const NoTagPicked: Story = { render: () => <Verbs /> };
