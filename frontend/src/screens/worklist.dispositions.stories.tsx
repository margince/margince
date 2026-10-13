// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { Panel, PanelBody } from "../design-system/panel";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import { DispositionVerbs } from "./worklist.dispositions";
// The screen's sheet, imported as worklist.tsx and brief.feed.tsx import it.
import "./worklist.css";
// And the ROW's own sheet, because `.worklist-row-acts` is the row's line
// rather than the screen's — a story mounting the verbs without it draws them
// as a bare inline flow with no gap between them.
import "./worklist.row.css";

// The ways a row can be PUT DOWN, on their own.
//
// These verbs have four arrangements and the row's stories show one. The
// others are the menu open, everything in the menu below 720px, and a
// judgement mid-write.

type WorklistItem = components["schemas"]["WorklistItem"];

// A buyer waiting on a colleague's answer, carrying all four judgements the
// contract offers. Which verbs a reader may use is a server rule. A fixture
// offering fewer is a picture of a narrower row than the product draws. On the
// reader's own row the server leaves out Not mine.
const WAITING: WorklistItem = {
  id: "01a05500-0000-7000-8000-0000000000a1",
  source: "customer_waiting",
  category: "customer_waiting",
  level: 1,
  consequence: "buyer_waits",
  title: "Re: pricing for the retrofit",
  because: [],
  actions: [],
  dispositions: ["handled", "snooze", "not_mine", "not_sales"],
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
                are the ones a reader actually meets. No call to action beside
                them: these are the row's quieter half, and the lane's answer
                leads the line — which is the row's own business. */}
            <div className="worklist-row-acts">
              <Story />
            </div>
          </PanelBody>
        </Panel>
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof DispositionVerbs>;

// The menu, opened. Shared by the stories of the open menu, which differ only
// in their theme, so the gesture is spelled once.
const openTheMenu: Story["play"] = async ({ canvasElement }) => {
  const canvas = within(canvasElement);
  await userEvent.click(
    await canvas.findByRole("button", { name: "Remove from list" }),
  );
};

/**
 * Done, and the menu holding the judgements that only move the row.
 *
 * Done is the one judgement that finishes the row, so it stays a button. The
 * rest are one tab stop: a band of four buttons wrapped every row onto a second
 * line of verbs.
 */
export const TheFourJudgements: Story = {
  args: { item: WAITING },
  render: (args) => {
    settleAtOnce();
    return <DispositionVerbs {...args} />;
  },
};

/** The same verbs in dark, where the ghost verbs sit on the darker panel. */
export const TheFourJudgementsDark: Story = {
  args: { item: WAITING },
  globals: { theme: "dark" },
  render: (args) => {
    settleAtOnce();
    return <DispositionVerbs {...args} />;
  },
};

/**
 * Not mine, focused in the open menu: the hint says it leaves only this
 * reader's list and the record keeps its owner. Focus rather than hover, so a
 * keyboard reader's route is the one drawn.
 */
export const NotMineExplainsItself: Story = {
  args: { item: WAITING },
  render: (args) => {
    settleAtOnce();
    return <DispositionVerbs {...args} />;
  },
  play: async (context) => {
    await openTheMenu(context);
    (await screen.findByRole("button", { name: "Not mine" })).focus();
  },
};

/**
 * The menu open: Snooze, Not mine, Not a customer, then the snooze answers a
 * plain press cannot give.
 *
 * Each snooze line is a whole sentence, "Snooze for 3 days". A bare span under
 * "Snooze" is a fragment whose verb the reader has to reconstruct.
 */
export const HowLongToPutItDownFor: Story = {
  args: { item: WAITING },
  render: (args) => {
    settleAtOnce();
    return <DispositionVerbs {...args} />;
  },
  play: openTheMenu,
};

/**
 * The same menu in dark, where the panel is the lighter of the two surfaces.
 * Its lines carry no ground, so `--bgElevated`, the hairline and
 * `--shadow-pop` are all that say where it ends.
 */
export const HowLongToPutItDownForDark: Story = {
  args: { item: WAITING },
  globals: { theme: "dark" },
  render: (args) => {
    settleAtOnce();
    return <DispositionVerbs {...args} />;
  },
  play: openTheMenu,
};

/**
 * The same judgements at 390px, where the band is a MENU.
 *
 * Below 720px Done joins the menu too: the width belongs to the work and a
 * swipe carries the fast path. Opened, because its lines are the whole
 * arrangement. `uat-phone` makes the capture gate draw it at 390px.
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
      await canvas.findByRole("button", { name: "Remove from list" }),
    );
  },
};

/**
 * One judgement written, the rest standing down.
 *
 * Every judgement refuses a second press while a write is out: Done followed by
 * "Not a customer" would be two contradictory answers. This PUT never answers,
 * so the state stays on screen. The pressed verb keeps focus and turns busy;
 * the menu's lines are only disabled.
 */
export const AJudgementBeingWritten: Story = {
  args: { item: WAITING },
  render: (args) => {
    neverSettles();
    return <DispositionVerbs {...args} />;
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByRole("button", { name: "Done" }));
  },
};
