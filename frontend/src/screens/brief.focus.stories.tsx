// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { Panel } from "../design-system/panel";
import { taskRow, waitingEmailRow } from "./brief.fixtures";
import { Triage } from "./brief.focus";
import { installFetchStub, StoryProviders } from "./story-utils";
import type { WorklistItem } from "./worklist.queries";

// THE FOCUS PANEL'S INSIDE: the card in hand on the stack of what is still
// behind it, and the ranked column beside it.
//
// What to check in each frame:
//   · the card reads label, work, whose row it is, verbs — the verbs across
//     its floor, the set-asides leading and the prepared move closing;
//   · the card fills the panel's height, so the air is inside it rather than
//     a hole beside the queue;
//   · the edges of two more cards show under it, and only as many as there
//     are rows behind;
//   · pressing a row in the column puts it in hand, and the card comes up off
//     the stack (the animation is CSS; a frame catches its end state).
//
// Both themes: the stack's edges, the card's ground and the accent band above
// it are all `color-mix()` over canonical tokens and re-resolve on the flip.

const meta: Meta<typeof Triage> = {
  title: "Shell/Home focus",
  component: Triage,
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj<typeof Triage>;

/** A day with a customer waiting at the top and five more behind it. */
function aDay(): WorklistItem[] {
  return [
    waitingEmailRow(),
    ...[1, 2, 3, 4, 5].map((n) =>
      taskRow(`followup-${n}`, `Follow up on customer commitment ${n}`),
    ),
  ];
}

/** The panel around it, so the card is judged in the width it is drawn at. */
function panel(rows: WorklistItem[], start = 0) {
  return () => {
    installFetchStub({});
    return (
      <StoryProviders>
        <Chosen rows={rows} start={start} />
      </StoryProviders>
    );
  };
}

function Chosen({
  rows,
  start,
}: Readonly<{ rows: WorklistItem[]; start: number }>) {
  const [at, setAt] = useState(start);
  return (
    <div id="brief-today">
      <Panel tone="accent" className="brief-focus-panel" title="Focus">
        <Triage
          rows={rows}
          at={at}
          lead={rows[at]}
          onChoose={(item) => setAt(rows.indexOf(item))}
          onOpenEmail={() => undefined}
          onContext={() => undefined}
        />
      </Panel>
    </div>
  );
}

/** The first row in hand: a customer waiting, with five behind it. */
export const InHand: Story = { render: panel(aDay()) };

/** Pressing the fourth row puts it in hand and says where it stands. */
export const Pressed: Story = {
  render: panel(aDay()),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: /Follow up on customer commitment 3/,
      }),
    );
    await expect(canvas.getByText("4 of 6")).toBeTruthy();
  },
};

/** The last row of a short day: one behind it, so the stack is one edge deep. */
export const NothingBehindIt: Story = {
  render: panel([waitingEmailRow(), taskRow("one", "Call the buyer")], 1),
};
