// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Badge, Button } from "../../design-system/atoms";
import { StoryProviders } from "../story-utils";
import { FoundMove, TodayPanel, TodoRow } from "./today";

// WHAT NEEDS A PERSON TODAY, on its own — the pane four record pages draw, so
// what it claims has to be right on all four at once.
//
// The claim is authorship: the moves are what the agent found and the drafts
// are what it would write, so the pane is indigo and its head says
// "AI-assisted" in words. What the stories are for is the DIVIDE inside it —
// a found move at the pane's loudest weight, and under it the record's own
// to-dos, where only the verb the agent performs carries the hue. A to-do
// somebody else owes is the account's, not the machine's, and its verb is a
// plain outlined one; the two rows side by side are the only way to see that
// the tint still means something.
//
// The head is `Panel`'s own, and the fixture below gives it the widest thing
// it has to hold: a title, the disclosure and the verb on one band, with what
// the day counts down to and what it was read from in the band UNDER the rows.
// Crammed into the head those three read as one long line and the badge broke
// in half, so the story carries the full foot rather than a single chip.
//
// Check both themes. Every indigo here is a color-mix() that lifts on dark,
// the badge and the tinted verb included. Check the narrow width too: the
// story's own frame is the desktop measure, and the pane draws at 390px on
// four record pages, where the title takes the second line and the head's
// actions stay whole.

const meta: Meta<typeof TodayPanel> = {
  title: "Records/Record reading/What needs you",
  component: TodayPanel,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof TodayPanel>;

// What the day counts down to and what it was read from, exactly as
// companytoday.tsx assembles it: a commitment badge, then the read's own
// provenance and what it covered.
const foot = (
  <>
    <Badge tone="warn">1 overdue</Badge>
    <span className="co-scan-foot">
      <Badge tone="ai">Margince</Badge>
      <span className="co-row-meta">Read 14 exchanges and 2 deals</span>
    </span>
  </>
);

function Pane({ width = 720 }: Readonly<{ width?: number }>) {
  return (
    <StoryProviders>
      <div style={{ maxWidth: width }}>
        <TodayPanel onOpenTasks={() => {}} footer={foot}>
          <FoundMove
            when="06:52"
            title="Send the breakdown Lena promised on 5 August."
            why="Lena promised this breakdown in the 5 August session and it never went out. The sheet is generated, so this can go on its own."
            action={
              <Button small variant="ai">
                Draft it
              </Button>
            }
            defer={{ onDefer: () => {} }}
          />
          {/* The agent's own verb: it writes the draft, so the chip is tinted. */}
          <TodoRow
            who="Lena Fischer"
            title="Send the promised line-item 3 breakdown"
            meta="Lena Fischer · promised 05/08"
            due={{ label: "19 days late", tone: "danger" }}
            verb={{ label: "Draft", onAct: () => {}, byMargince: true }}
          />
          {/* The same row shape for a commitment nobody automated: the verb
              opens the record and the hue stays out of it. */}
          <TodoRow
            who="Tomas Beck"
            title="Confirm the depot slot with facilities"
            meta="Tomas Beck · due 12/08"
            due={{ label: "in 3 days" }}
            verb={{ label: "Open", onAct: () => {} }}
          />
        </TodayPanel>
      </div>
    </StoryProviders>
  );
}

export const FoundAndOwed: Story = { render: () => <Pane /> };

export const FoundAndOwedDark: Story = {
  ...FoundAndOwed,
  globals: { theme: "dark" },
};

// The pane at phone measure, which is where its head has least room: the title
// takes the second line and the disclosure beside the verb stays whole. The
// same pane the four record pages draw — none of them narrows it further.
export const FoundAndOwedNarrow: Story = { render: () => <Pane width={390} /> };

export const NothingNeedsYou: Story = {
  // The honest quiet answer, which is a reading rather than an empty list —
  // and still the machine's, which is why the head keeps its claim.
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 720 }}>
        <TodayPanel onOpenTasks={() => {}} />
      </div>
    </StoryProviders>
  ),
};

export const StillReading: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 720 }}>
        <TodayPanel state="loading" />
      </div>
    </StoryProviders>
  ),
};

export const ReadFailed: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 720 }}>
        <TodayPanel state="failed" />
      </div>
    </StoryProviders>
  ),
};
