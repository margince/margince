// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../i18n";
import type { DecisionDeckLabels } from "./decisiondeck";
import { DeckQueue, DeckSurface, StagingTray } from "./decisiondeck.frame";
import { Panel, PanelBody } from "./panel";

// WHERE THE DECK'S THREE PARTS GO, drawn without a queue behind them.
//
// The deck's own node shows these in motion; this one shows the two answers to
// "who places them" side by side — the deck drawing its own region, and a
// surface taking the toggle into its header band and the tray into its foot —
// plus the states of each part that a full deck can only reach one at a time.
//
// Both themes. The tray's floating shape is `--bgElevated` under `--shadow-pop`
// and its banded shape has none of that; a shadow that reads on paper is
// invisible on the dark surface, which is exactly why the band is the right
// shape inside a pane.
const meta: Meta = {
  title: "Design System/DecisionDeck frame",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <div style={{ maxWidth: 720 }}>
          <Story />
        </div>
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj;

const LABELS = {
  empty: "Nothing is waiting on you.",
  clearedTitle: "Deck clear",
  cleared: (count: number) => `${count} decisions sent`,
  clearedTime: () => "at 09:00",
  skipped: (count: number) => `${count} skipped`,
  edited: (count: number) => `${count} being edited`,
  staged: (count: number) => `${count} decisions staged`,
  commitNothingToSend: "Finish these",
  commit: "Send staged decisions",
  unstage: "Undo the last one",
} as unknown as DecisionDeckLabels;

const QUEUE = <p className="t-body">Three proposals, in whichever form.</p>;

const TRAY = (
  <StagingTray
    staged={[
      { id: "a", verdict: "accept" },
      { id: "b", verdict: "reject" },
      { id: "c", verdict: "skip" },
    ]}
    labels={LABELS}
    commitState="idle"
    commitRef={null}
    onCommit={() => undefined}
    onUnstage={() => undefined}
  />
);

// UNFRAMED: the deck names its own region and stacks the three parts itself.
export const ItsOwnRegion: Story = {
  render: () => (
    <DeckSurface
      deckLabel="Waiting on you"
      title="Waiting on you"
      toggle={<span className="t-caption">[ Deck | List ]</span>}
      queue={QUEUE}
      tray={TRAY}
    />
  ),
};

// FRAMED BY A PANEL: the toggle in the header band, the queue in a body that
// pays the pane's inset, the tray in the foot — edge to edge under a hairline,
// with no box of its own.
export const FramedByAPanel: Story = {
  render: () => (
    <DeckSurface
      deckLabel="Waiting on you"
      toggle={<span className="t-caption">[ Deck | List ]</span>}
      queue={QUEUE}
      tray={TRAY}
      frame={({ toggle, content, tray }) => (
        <Panel title="Waiting on you" titleAction={toggle} footer={tray}>
          {content === null ? null : <PanelBody>{content}</PanelBody>}
        </Panel>
      )}
    />
  ),
};

// EVERY CARD STAGED: the queue is null, so the pane is its head and its foot.
// The band says what is true by counting; "nothing is waiting on you" over a
// reader's own unsent verdicts would contradict it one line below.
export const NothingLeftToShow: Story = {
  render: () => (
    <DeckSurface
      deckLabel="Waiting on you"
      toggle={null}
      queue={null}
      tray={TRAY}
      frame={({ toggle, content, tray }) => (
        <Panel title="Waiting on you" titleAction={toggle} footer={tray}>
          {content === null ? null : <PanelBody>{content}</PanelBody>}
        </Panel>
      )}
    />
  ),
};

// The EARNED moment, which is the one reading this tier makes on its own: a
// tally of what it watched leave.
export const TheClearedPlate: Story = {
  render: () => (
    <DeckQueue
      state="ready"
      loadingLabel="Reading the proposals"
      labels={LABELS}
      tally={{ count: 4, at: Date.parse("2026-08-24T09:00:00.000Z") }}
      shown={QUEUE}
    />
  ),
};

// A read that never landed. The queue says so where the cards would be — an
// empty queue and a failed read are different facts.
export const TheReadFailed: Story = {
  render: () => (
    <DeckQueue
      state="failed"
      stateDetail={{ onRetry: () => undefined }}
      loadingLabel="Reading the proposals"
      labels={LABELS}
      tally={null}
      shown={QUEUE}
    />
  ),
};

// The tray mid-send and refused: the controls stand down while a commit is out,
// and a refusal keeps the tray — the verdicts in it are the only copy of a
// contact's answers.
export const TheTraySending: Story = {
  render: () => (
    <StagingTray
      staged={[
        { id: "a", verdict: "accept" },
        { id: "b", verdict: "accept" },
        { id: "c", verdict: "reject" },
      ]}
      labels={LABELS}
      commitState="sending"
      commitRef={null}
      onCommit={() => undefined}
      onUnstage={() => undefined}
    />
  ),
};
