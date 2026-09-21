// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { EmailEntry } from "./emailentry";
import { Panel, PanelRow } from "./panel";
import { SwipeRow } from "./swiperow";

// A row answered with the thumb, where there is no width for the verbs.
//
// Every frame here is about one question: can a reader put a row down with a
// gesture WITHOUT the gesture being what decides. The staged bar is the answer
// — a drag reveals what it would do, and the press that runs it is the
// reader's own.
//
// Framed at 390px, because that is the width the control exists for. At any
// wider size the caller draws its buttons and this never mounts.
const meta: Meta<typeof SwipeRow> = {
  title: "Design System/SwipeRow",
  component: SwipeRow,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <div style={{ maxWidth: 390 }}>
          <Panel>
            <PanelRow>
              <Story />
            </PanelRow>
          </Panel>
        </div>
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof SwipeRow>;

const work = <p className="t-body">Send the retrofit quote to Turbinenbau</p>;

// A drag of 90px, which is past the 56px threshold and horizontal enough to
// mean something. Written as pointer events rather than a drag helper because
// the control reads the two endpoints and nothing between them.
async function swipe(row: Element, from: number, to: number) {
  await userEvent.pointer([
    { keys: "[MouseLeft>]", target: row, coords: { x: from, y: 0 } },
    { target: row, coords: { x: to, y: 0 } },
    { keys: "[/MouseLeft]", target: row, coords: { x: to, y: 0 } },
  ]);
}

/** At rest: the row is the work, and nothing else. */
export const AtRest: Story = {
  args: {
    cancelLabel: "Keep it",
    end: { label: "Snooze", onAct: () => {} },
    start: { label: "Not mine", onAct: () => {} },
    children: work,
  },
};

/** Dragged forward: the snooze is staged and waiting to be pressed. */
export const SnoozeStaged: Story = {
  args: AtRest.args,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await swipe(canvas.getByTestId("swipe-row"), 0, 90);
    await expect(
      canvas.getByRole("button", { name: "Snooze" }),
    ).toBeInTheDocument();
  },
};

/** Dragged back: the other judgement, so a direction means something. */
export const NotMineStaged: Story = {
  args: AtRest.args,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await swipe(canvas.getByTestId("swipe-row"), 90, 0);
    await expect(
      canvas.getByRole("button", { name: "Not mine" }),
    ).toBeInTheDocument();
  },
};

/**
 * A drag that was meant as a scroll costs one tap.
 *
 * The frame that matters most: a reader moving down a list must be able to get
 * out of a stage they did not mean to make, without having filed anything.
 */
export const StagedThenKept: Story = {
  args: AtRest.args,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await swipe(canvas.getByTestId("swipe-row"), 0, 90);
    await userEvent.click(canvas.getByRole("button", { name: "Keep it" }));
    await expect(canvas.queryByRole("button", { name: "Snooze" })).toBeNull();
  },
};

/**
 * A ROW WITH A LINE THAT DOES NOT WRAP, at the width this control exists for.
 *
 * The frame the other five cannot be: their work is a paragraph, and a
 * paragraph wraps to any column. A real row often carries a line that does not
 * — `EmailEntry`'s subject and preview are `white-space: nowrap` with their own
 * ellipsis — and an unwrappable line is a MINIMUM width. This wrapper is a flex
 * item, so without `min-width: 0` (swiperow.css) its automatic minimum was that
 * whole line: it refused to shrink to the card, the panel clipped it, and every
 * box inside was laid out against a width the reader could not see.
 *
 * What to look for: the subject and the preview end in an ellipsis INSIDE the
 * card, and nothing reaches its right edge.
 */
export const AnUnwrappableLine: Story = {
  args: {
    ...AtRest.args,
    children: (
      <EmailEntry
        summary={{
          activity_id: "01a05500-0000-7000-8000-0000000000e1",
          subject: "Re: pricing for the retrofit and the spare parts schedule",
          preview:
            "Can you confirm the lead time before Friday? Procurement will not sign without it.",
          occurred_at: "2026-09-01T07:42:00Z",
          direction: "inbound",
          counterparty: "Kirsten Vogel",
          attachment_count: 1,
          move: "needs_reply",
          display_status: "team",
          version: 3,
        }}
        timestamp="01/09/2026, 07:42"
        onOpen={() => {}}
      />
    ),
  },
};

/** One side only: the other direction stages nothing rather than an empty bar. */
export const OneSideOnly: Story = {
  args: {
    cancelLabel: "Keep it",
    end: { label: "Snooze", onAct: () => {} },
    children: work,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await swipe(canvas.getByTestId("swipe-row"), 90, 0);
    await expect(canvas.queryByRole("status")).toBeNull();
  },
};
