// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { ActionRow } from "../design-system/actionrow";
import { Panel, PanelBody } from "../design-system/panel";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import { DispositionVerbs } from "./worklist.dispositions";
// The verbs' own layout: `.worklist-snooze-spans` draws the caret's panel as a
// list of choices rather than a grid of pills, and the row's `.action-row-*`
// groups come from the design system. Imported the same way every surface that
// mounts these verbs picks it up — worklist.tsx and brief.feed.tsx each do.
import "./worklist.css";

// The ways a row can be PUT DOWN, on their own.
//
// Covered here rather than only through the row because these three verbs have
// four arrangements and the row's stories show one of them: the split control
// closed (the common press), its chooser open (the four spans), the whole band
// folded into a menu below 720px, and a judgement mid-write with the rest of
// the band standing down. Three of those are unreachable from a wide desktop
// canvas, which is how the folded arrangement lost the snooze spans for a
// release with nothing on screen to notice.

type WorklistItem = components["schemas"]["WorklistItem"];

// A buyer waiting on an answer, carrying all three judgements the contract
// offers. Every one of them, because which verbs a reader may use is a SERVER
// rule and a fixture offering two of three is a picture of a narrower row than
// the product draws.
const WAITING: WorklistItem = {
  id: "01a05500-0000-7000-8000-0000000000a1",
  source: "customer_waiting",
  category: "customer_waiting",
  level: 1,
  consequence: "buyer_waits",
  title: "Re: pricing for the retrofit",
  because: [],
  actions: [],
  dispositions: ["snooze", "not_mine", "not_sales"],
};

// The write these verbs make, answered at once. A story that left the PUT to
// the empty-page fallback would render the same buttons and confirm nothing.
function settleAtOnce() {
  installFetchStub({
    [`PUT /activities/${WAITING.id}/disposition`]: () =>
      jsonResponse(null, 204),
  });
}

// The same write, never answering — which is the only way to draw the band
// while one judgement is in flight. `pending` is not a prop: it is
// `useSetDisposition().isPending`, so the state exists on screen only for as
// long as a request is out.
function neverSettles() {
  installFetchStub({
    [`PUT /activities/${WAITING.id}/disposition`]: () =>
      new Promise<Response>(() => {}),
  });
}

const meta: Meta<typeof DispositionVerbs> = {
  title: "Records/Worklist/Disposition verbs",
  component: DispositionVerbs,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Panel title="What to do next">
          <PanelBody>
            {/* The container the row puts them in, so the gap and the wrapping
                are the ones a reader actually meets. No primary: this band is
                the row's quieter half, and the lane's call to action is the
                row's own business. */}
            <ActionRow className="worklist-row-acts">
              <Story />
            </ActionRow>
          </PanelBody>
        </Panel>
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof DispositionVerbs>;

// The caret, pressed. Shared by the two stories of the open chooser rather than
// spelled in each: they differ in the THEME the panel is drawn in, and a second
// copy of the gesture is a second thing to fix when the caret is renamed.
const openTheChooser: Story["play"] = async ({ canvasElement }) => {
  const canvas = within(canvasElement);
  await userEvent.click(
    await canvas.findByRole("button", { name: "For how long" }),
  );
};

/**
 * The three judgements, and the snooze as ONE control in two halves.
 *
 * The press is the common case — a rep reaching for "not today" most often
 * means tomorrow — and the caret is the rare one. Closed here, because closed
 * is what a reader meets: what the split has to show at rest is that it reads
 * as one control with a seam rather than as a verb and a chevron beside it.
 */
export const TheThreeJudgements: Story = {
  args: { item: WAITING },
  render: (args) => {
    settleAtOnce();
    return <DispositionVerbs {...args} />;
  },
};

/**
 * The chooser behind the caret: the four answers a plain press cannot give.
 *
 * Opened by `play`, because a popover closed is a story that shows nothing
 * about what it holds. Each line is a whole sentence — "Snooze for 3 days" —
 * since a bare span standing under a caret is a fragment whose verb the reader
 * has to reconstruct from the button they pressed, and they read as lines of a
 * list rather than as a stack of pills.
 */
export const HowLongToPutItDownFor: Story = {
  args: { item: WAITING },
  render: (args) => {
    settleAtOnce();
    return <DispositionVerbs {...args} />;
  },
  play: openTheChooser,
};

/**
 * The same chooser in DARK, where the panel is the LIGHTER of the two surfaces.
 *
 * Its lines carry no ground and no border of their own — `worklist.css` strips
 * both so four sentences read as a list rather than as four pills — so the only
 * things saying where the chooser ends are the panel's `--bgElevated`, its
 * hairline and `--shadow-pop`. In light the panel is a near-white card over a
 * grey ground and its own lightness does most of that work. In dark that step
 * inverts and the shadow is a separately tuned token, so this is the theme
 * where a panel of groundless lines can lose its edge, and the resting
 * arrangement is what has to be looked at rather than any one line's state.
 */
export const HowLongToPutItDownForDark: Story = {
  args: { item: WAITING },
  globals: { theme: "dark" },
  render: (args) => {
    settleAtOnce();
    return <DispositionVerbs {...args} />;
  },
  play: openTheChooser,
};

/**
 * The same three judgements at 390px, where the band is a MENU.
 *
 * Below 720px the buttons are gone rather than smaller — the width belongs to
 * the work, and a swipe carries the fast path — so one overflow menu is what
 * keeps every judgement reachable by key. Opened, because the menu's lines are
 * the whole arrangement: the three verbs AND the snooze spans, which live in
 * the caret's popover above this width and as lines of their own here.
 *
 * `uat-phone` is what makes the capture gate drive the browser to 390px.
 * Without it the frame is captured at the harness's own width and draws the
 * band, which is the opposite of what this story is named for.
 */
export const FoldedToAMenu: Story = {
  args: { item: WAITING },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: (args) => {
    settleAtOnce();
    return <DispositionVerbs {...args} />;
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Take off the list" }),
    );
  },
};

/**
 * One judgement written, the rest of the band standing down.
 *
 * The whole band refuses a second press while a write is out, and it has to:
 * these are alternative answers to one row, so a rep who presses "Not mine"
 * and then "Not a customer" has filed two contradictory judgements on one
 * customer's message. The state is reachable only while a request is in
 * flight, which is why this story's PUT never answers.
 *
 * TWO refusals, not one, and which is which is the thing to look at. The verb
 * the reader PRESSED is the busy one — it keeps its focus and turns a mark, so
 * the wait is announced from where the reader is standing — and the answers
 * beside it are merely disabled, because they started nothing. One control
 * drawn busy per row: a band that marked every answer would claim a write from
 * each of them, and only one went out.
 *
 * The caret is refused as well, and it is the sibling case rather than the busy
 * one: it starts no write, and every line behind it would refuse the press for
 * as long as this one is out, so opening it could only hand the reader four
 * dead choices and a panel their focus cannot enter. A chooser already OPEN
 * when the write starts is left open — the line they pressed is inside it,
 * holding their place — which is the asymmetry `Popover.disabled` exists for:
 * it blocks the opening and never the closing.
 */
export const AJudgementBeingWritten: Story = {
  args: { item: WAITING },
  render: (args) => {
    neverSettles();
    return <DispositionVerbs {...args} />;
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Not mine" }),
    );
  },
};
