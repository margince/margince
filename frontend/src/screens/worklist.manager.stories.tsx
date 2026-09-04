// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { en } from "../i18n/en";
import { jsonResponse, StoryProviders } from "./story-utils";
import { CoachControl, OwnerPicker, ReassignControl } from "./worklist.manager";
import type { WorklistItem } from "./worklist.queries";

// THE THREE THINGS A LEAD DOES with somebody else's queue: open it, hand one
// task on, and leave a note about the day.
//
// Each verb rests as a single ghost button and opens its own form on the press,
// which is what the frames below are about — the resting row is what a reader
// meets, and the open form is where the choice actually is. The coach form is
// the one worth looking at: the KIND is a control and the note is a plain
// field, because the recipient reads a sentence the product wrote either way
// and the coach adds to it rather than composing from nothing.
//
// Read every frame in BOTH themes with the toolbar's Theme control.

const LENA = "00000000-0000-4000-8000-000000000001";

// Everyone the reader may hand work to. The agent seat is in the roster on
// purpose: `useAssigneeOptions` drops it, because the reassign endpoint checks
// existence rather than seat kind and would park the task where no person's
// queue shows it.
const roster = {
  data: [
    { id: LENA, display_name: "Lena Fischer" },
    { id: "00000000-0000-4000-8000-000000000002", display_name: "Marc Weber" },
    {
      id: "00000000-0000-4000-8000-00000000000a",
      display_name: "Margince",
      is_agent: true,
    },
  ],
  page: { next_cursor: null, has_more: false },
};

function stubRoster() {
  globalThis.fetch = (async (): Promise<Response> =>
    jsonResponse(roster)) as typeof fetch;
}

function frame(children: React.ReactNode) {
  stubRoster();
  return (
    <StoryProviders>
      <div style={{ maxWidth: 420 }}>{children}</div>
    </StoryProviders>
  );
}

const aTask = { id: "act-1", title: "Answer Kirsten" } as WorklistItem;

const meta: Meta<typeof CoachControl> = {
  title: "Records/Worklist/A lead's verbs",
  component: CoachControl,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof CoachControl>;

/** Whose queue the page is answering. "My own day" is the first entry rather
 *  than a blank, so the dial always names what it is showing. */
export const WhoseQueue: Story = {
  render: () => frame(<OwnerPicker owner="" onOwner={() => {}} />),
};

/** Both verbs at rest — one ghost button each, and nothing on screen yet about
 *  the choice behind them. */
export const TheVerbsAtRest: Story = {
  render: () =>
    frame(
      <>
        <ReassignControl item={aTask} owner={LENA} />
        <CoachControl owner={LENA} />
      </>,
    ),
};

/** Handing the task on. The confirm refuses until somebody is picked — a
 *  precondition the reader can meet, which is what `disabled` is for. */
export const HandingItOn: Story = {
  render: () => frame(<ReassignControl item={aTask} owner={LENA} />),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      within(canvasElement).getByRole("button", {
        name: en["worklist.manager.reassign"],
      }),
    );
  },
};

/** The note on somebody's day: the kind as a segmented control, the words as a
 *  plain field under it. */
export const LeavingANote: Story = {
  render: () => frame(<CoachControl owner={LENA} />),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      within(canvasElement).getByRole("button", {
        name: en["worklist.manager.coach"],
      }),
    );
  },
};
