// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";

import { RowTags } from "./rowtags";

// The list-row counterpart to the record panel's strip. Two words rather than
// the panel's four, because this one shares its row with every other column —
// and it never wraps, since a cell that grows a second line pushes every row
// below it down.

const meta: Meta<typeof RowTags> = {
  title: "Design System/RowTags",
  component: RowTags,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof RowTags>;

type Tone = "teal" | "amber" | "rose" | "slate";

const tag = (id: string, name: string, color?: Tone) => ({
  tag_id: id,
  name,
  color: color ?? null,
});

/** The common row: one or two words, drawn whole. */
export const WithinTheCap: Story = {
  render: () => <RowTags tags={[tag("t-1", "Key Account", "amber")]} />,
};

/** Past the cap the rest are counted, and named in the title so a reader is
 * given an answer rather than a puzzle. */
export const Overflowing: Story = {
  render: () => (
    <RowTags
      tags={[
        tag("t-1", "Key Account", "amber"),
        tag("t-2", "Churn Risk", "rose"),
        tag("t-3", "EV programme", "teal"),
        tag("t-4", "DACH", "slate"),
      ]}
    />
  ),
};

/** A row with no tags draws NOTHING. A per-row empty state would repeat one
 * sentence fifty times down a page. */
export const NoTags: Story = {
  render: () => <RowTags tags={[]} />,
};
