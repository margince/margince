// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ListChangeSummary } from "./listchanges";
import { changesSinceVisit } from "./lists.fixtures";
import type { List } from "./lists.queries";
import { StoryProviders } from "./story-utils";

// What changed on a Live List since the reader's last visit, as one sentence.
// The newest records are named, and a record type with a page opens from here.
const meta: Meta = {
  title: "Records/List changes",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

type Summary = NonNullable<List["changes_since_visit"]>;

function sentence(summary: Summary, opens: boolean) {
  return () => (
    <StoryProviders>
      <ListChangeSummary summary={summary} onOpen={opens ? fn() : undefined} />
    </StoryProviders>
  );
}

// Records joined and left and the filter changed; each named record opens.
export const JoinedAndLeft: Story = {
  render: sentence(changesSinceVisit, true),
};

// A record type with no page of its own names its records as plain text.
export const PlainNames: Story = { render: sentence(changesSinceVisit, false) };

// A long name with no page of its own wraps like the rest of the sentence.
export const LongPlainNamePhone: Story = {
  render: sentence(
    {
      ...changesSinceVisit,
      joined: {
        ...changesSinceVisit.joined,
        records: [
          {
            entity_id: "01a0f000-0000-7000-8000-000000000007",
            name: "Norddeutsche Stahl- und Metallbau Gesellschaft für Hallen und Fassaden",
          },
        ],
      },
    },
    false,
  ),
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// The reader came back and nothing moved.
export const NothingMoved: Story = {
  render: sentence(
    {
      since: "2026-09-28T17:00:00Z",
      joined: { count: 0, records: [] },
      left: { count: 0, records: [] },
      filter_changes: 0,
    },
    true,
  ),
};

export const JoinedAndLeftDark: Story = {
  ...JoinedAndLeft,
  globals: { theme: "dark" },
};

// At 390px the sentence wraps, and no bracket or comma is left on a line
// without the name it belongs to.
export const JoinedAndLeftPhone: Story = {
  ...JoinedAndLeft,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
