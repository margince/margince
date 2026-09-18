// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Badge, Button } from "../../design-system/atoms";
import { StoryProviders } from "../story-utils";
import { MomentEvidence } from "./momentevidence";
import { FoundMove, TodayPanel, TodoRow } from "./today";

// WHAT NEEDS A CONTACT TODAY, on its own — the pane four record pages draw, so
// what it claims has to be right on all four at once.
//
// The claim is authorship, and the pane's indigo is where it is made. What the
// stories are for is the DIVIDE inside it — a found move at the pane's loudest
// weight, carrying its own byline, and under it the record's own to-dos, where
// only the verb the agent performs takes the hue. A to-do somebody else owes is
// the account's, not the machine's, and its verb is a plain outlined one; the
// two rows side by side are the only way to see that the tint still means
// something, and that nothing on the head claims both.
//
// The head is `Panel`'s own: a title and the way to the record's own list, with
// what the day counts down to and what it was read from in the band UNDER the
// rows. The fixture below carries that full foot rather than a single chip,
// because three blocks wedged beside the title left none of them whole.
//
// Check both themes. Every indigo here is a color-mix() that lifts on dark, the
// foot's badges and the tinted verb included. Check the phone story too: the
// pane draws at 390px on four record pages, and it is the only width where the
// move's verbs sit under its claim rather than beside it.

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
    <Badge tone="warning">1 overdue</Badge>
    <span className="co-scan-foot">
      <Badge tone="ai">Margince</Badge>
      <span className="co-row-meta">Read 14 exchanges and 2 deals</span>
    </span>
  </>
);

function Pane({
  width = 720,
  kicker,
  taskTitle = "Send the promised line-item 3 breakdown",
}: Readonly<{
  width?: number;
  kicker?: string;
  taskTitle?: string;
}>) {
  return (
    <StoryProviders>
      <div style={{ maxWidth: width }}>
        <TodayPanel onOpenTasks={() => {}} footer={foot}>
          <FoundMove
            when="06:52"
            kicker={kicker}
            title="Send the breakdown Lena promised on 5 August."
            why="Lena promised this breakdown in the 5 August session and it never went out. The sheet is generated, so this can go on its own."
            // The fourth line of the left column, and the reason it is here:
            // byline, ask, reason and basis are the interval this pane is
            // judged on, and a fixture that stops at the reason cannot show
            // whether the evidence still reads as part of the case for the move.
            basis="The 5 August session note, and the thread the promise was made in."
            // Two verbs and a defer, in the one column FoundMove itself owns:
            // the leading verb ai-tinted, the second and the defer both
            // ghost, every one the same width and stacked, never a verb
            // beside the defer in a row of its own.
            action={
              <>
                <span className="today-verb">
                  <Button variant="ai">Draft it</Button>
                </span>
                <span className="today-verb">
                  <Button variant="ghost">Open the thread</Button>
                </span>
              </>
            }
            defer={{ onDefer: () => {} }}
          />
          {/* The agent's own verb: it writes the draft, so the chip is tinted. */}
          <TodoRow
            who="Lena Fischer"
            title={taskTitle}
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

// The rule the record was read against, as the byline's second clause. It
// qualifies the authorship claim — read against WHAT — so it sits beside
// "Margince suggests" rather than over the ask, which is the move itself.
export const FoundMoveWithKicker: Story = {
  render: () => <Pane kicker="Promise overdue" />,
};

// The pane at phone measure, and the only story where the move's verbs sit
// UNDER its claim instead of opposite it — the two-column move and the to-do's
// three-part line both turn here, so this is where a regression in either one
// shows. The frame is the VIEWPORT rather than a capped div, because both folds
// are media queries: a narrow box inside a desktop window still draws the wide
// layout, which is exactly the false pass this story used to give.
//
// The to-do carries a title no phone column can hold on one line: it wraps to
// two and then ends in an ellipsis, rather than pushing the rows under it off
// the screen.
export const FoundAndOwedNarrow: Story = {
  globals: { viewport: { value: "phone" } },
  // And the tag beside it, because the two answer different readers: the global
  // moves the manager's frame for a person opening the catalog, and the tag
  // moves the BROWSER for `make fe-uat`, which loads the iframe directly and
  // never runs the manager.
  tags: ["uat-phone"],
  render: () => (
    <Pane taskTitle="Send the promised line-item 3 breakdown for the Hamburg retrofit, including the depot slot Tomas confirmed" />
  ),
};

// A verb the record cannot back yet: `reason` bars the press and states why
// under the button, in the same column a working verb draws in — never a
// second control and never a caption floating loose beside it.
export const FoundMoveRefused: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 720 }}>
        <TodayPanel onOpenTasks={() => {}}>
          <FoundMove
            when="06:52"
            title="Send the breakdown Lena promised on 5 August."
            why="Lena promised this breakdown in the 5 August session and it never went out."
            action={
              <span className="today-verb">
                <Button
                  variant="ai"
                  reason="Lena has no address on file to send this to."
                >
                  Draft it
                </Button>
              </span>
            }
          />
        </TodayPanel>
      </div>
    </StoryProviders>
  ),
};

// The three things that can each push this row off a phone, in one story,
// because on the live record they arrive together.
//
// The basis is the REAL `MomentEvidence`, not a sentence standing in for it:
// what a source names is a record of ours, so its label is an email subject and
// nothing about its length is this pane's to choose. It has to give way inside
// its own row.
//
// The second verb is REFUSED, which is what the story above could not show.
// `Button`'s `reason` wraps the control in `.btn-with-reason`, and that box is
// `contain: inline-size` so a long sentence cannot widen the verb column — but
// a contained box also measures as nothing in a flex line, so the line never
// wrapped and the refused verb and its sentence walked off the panel. A verb
// per line is what holds it.
export const FoundMoveNarrowRefusedAndLongBasis: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => (
    <StoryProviders>
      <TodayPanel onOpenTasks={() => {}}>
        <FoundMove
          when="06:52"
          kicker="Gone quiet"
          title="No reply for 131 days"
          why="Follow up before the thread goes cold."
          basis={
            <MomentEvidence
              evidence={[
                {
                  type: "activity",
                  id: "0f1c6b4e-2f4a-4a5c-9f0e-7d2b8a3c1e55",
                  label: "Timeout behoben - Sammelabfrage ist live",
                },
              ]}
              onOpen={() => {}}
            />
          }
          action={
            <>
              <span className="today-verb">
                <Button variant="ai">Draft a follow-up</Button>
              </span>
              <span className="today-verb">
                <Button
                  variant="ghost"
                  reason="Asking a colleague for context isn't available yet"
                >
                  Ask for context
                </Button>
              </span>
            </>
          }
        />
      </TodayPanel>
    </StoryProviders>
  ),
};

// The ordinary case: one verb, no defer. The column is exactly as wide as
// this one button, never wider for a second verb that is not there.
export const FoundMoveOneVerb: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 720 }}>
        <TodayPanel onOpenTasks={() => {}}>
          <FoundMove
            when="06:52"
            title="Send the breakdown Lena promised on 5 August."
            why="Lena promised this breakdown in the 5 August session and it never went out."
            action={
              <span className="today-verb">
                <Button variant="ai">Draft it</Button>
              </span>
            }
          />
        </TodayPanel>
      </div>
    </StoryProviders>
  ),
};

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
