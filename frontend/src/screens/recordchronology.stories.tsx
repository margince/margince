// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { ChronologyFilter, type TimelineFilter } from "./recordchronology";
import { StoryProviders } from "./story-utils";

// The row of cuts sitting above a record's chronology: All first, because
// that is where a reader starts. Conversations is opt-in per page rather than
// a base cut: it only shows where the page has wired a renderer for it, so
// the two stories below are the two rows a caller may actually offer.

// Live, so the pressed pill is the reader's own choice rather than a fixed
// prop: `onFilter` is the point of the row.
function Live(
  props: Readonly<{ initial: TimelineFilter; conversations?: boolean }>,
) {
  const [filter, setFilter] = useState<TimelineFilter>(props.initial);
  return (
    <StoryProviders>
      <ChronologyFilter
        filter={filter}
        conversations={props.conversations}
        onFilter={setFilter}
      />
    </StoryProviders>
  );
}

const meta: Meta<typeof Live> = {
  title: "Records/Chronology filter",
  component: Live,
};
export default meta;
type Story = StoryObj<typeof Live>;

// The base row: All, Activities, Changes. No Conversations pill, because the
// page has wired no renderer for that cut.
export const WithoutConversations: Story = {
  render: () => <Live initial="all" />,
};

// The four cuts, Conversations sitting between the whole and the parts, and
// pressed, so the row reads with a narrower cut selected rather than always
// on the default.
export const WithConversationsSelected: Story = {
  render: () => <Live initial="conversations" conversations />,
};
