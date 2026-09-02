// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Panel, PanelBody } from "../../design-system/panel";
import { StoryProviders } from "../story-utils";
import { CallCard, RecordReading, RecordReadingPair } from "./reading";
import { FoundMove, TodayPanel, TodoRow } from "./today";

// One reading, in parts, as every record page draws it: the call, the day's
// work, and the two reference sections a reader consults rather than reads.
// The stories are the shapes it takes — a record with a move to make, and a
// record with nothing waiting on anyone.
const meta: Meta<typeof RecordReading> = {
  title: "Records/Record reading",
  component: RecordReading,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof RecordReading>;

function Reference({ title }: Readonly<{ title: string }>) {
  return (
    <Panel title={title}>
      <PanelBody>
        <p className="t-small">
          A reference section a reader consults rather than reads.
        </p>
      </PanelBody>
    </Panel>
  );
}

export const WithAMove: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 960 }}>
        <RecordReading>
          <CallCard
            name="Frédéric de Gombert"
            standing={{ label: "Your move", tone: "warn" }}
            because="You owe Frédéric the line-item 3 breakdown. Promised on 5 August, 19 days ago."
            restsOn={[
              {
                key: "promise",
                quote:
                  "I'll send you the breakdown of the proportional calculation for line item 3 this week.",
                from: "Meeting transcript · 05/08/2026 · Lena Fischer",
              },
              {
                key: "mail",
                quote:
                  "No outgoing email with an attachment to him since 01/08/2026.",
                from: "Mailbox scan · 24/08/2026",
              },
            ]}
          />
          <TodayPanel onOpenTasks={() => {}}>
            <FoundMove
              when="06:52"
              title="Send the breakdown Lena promised on 5 August."
              why="Lena promised this breakdown in the 5 August session and it never went out. The sheet is generated, so this can go on its own."
              action={<button type="button">Create draft</button>}
              defer={{ onDefer: () => {} }}
            />
            <TodoRow
              who="Lena Fischer"
              title="Send the promised line-item 3 breakdown"
              meta="Lena Fischer · promised 05/08"
              due={{ label: "19 days late", tone: "danger" }}
              verb={{ label: "Draft", onAct: () => {}, byMargince: true }}
            />
          </TodayPanel>
          <RecordReadingPair>
            <Reference title="What was said lately" />
            <Reference title="Commitments" />
          </RecordReadingPair>
        </RecordReading>
      </div>
    </StoryProviders>
  ),
};

// The same reading on the dark ground: the indigo band, the dashed spine and
// the move row are all color-mix() of tokens that lift with the dark accent,
// and a surface can be right in light and wrong here.
export const WithAMoveDark: Story = {
  ...WithAMove,
  globals: { theme: "dark" },
};

export const NothingWaiting: Story = {
  // The quiet answer: the call says their move, and the day's work says so in
  // one sentence rather than drawing an empty list.
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 960 }}>
        <RecordReading>
          <CallCard
            name="Frédéric de Gombert"
            standing={{ label: "Their move", tone: "calm" }}
            because="He replied on 20 August and the contract is with their legal team."
          />
          <TodayPanel />
        </RecordReading>
      </div>
    </StoryProviders>
  ),
};

export const StillReading: Story = {
  // The read behind the rows is in flight: the day's work names itself and
  // waits, and no call is drawn — a head holding a spinner where the verdict
  // goes is the reading claiming to have reached one.
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 960 }}>
        <RecordReading>
          <TodayPanel state="loading" />
        </RecordReading>
      </div>
    </StoryProviders>
  ),
};
