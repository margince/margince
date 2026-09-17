// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { Panel, PanelBody } from "../design-system/panel";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import {
  BriefAct,
  BriefSetAsides,
  useBriefAnswer,
} from "./worklist.briefverbs";
import type { WorklistItem } from "./worklist.queries";
// The row's own line: `.worklist-row-acts` carries the trailing alignment, the
// wrapping and the interval, so without it the three draw as a bare inline run.
import "./worklist.row.css";

// A BRIEF ITEM'S THREE VERBS, answered where the row is ranked.
//
// The queue's own stories draw them through the row, which is the right way to
// see a row and the wrong way to see these: what differs below is which verbs
// the server offered and whether the set has been ANSWERED, and a whole row
// buries both under the rank, the kind, the title and the facts. They have no
// container of their own either — acting is the row's call to action and the
// other two are ways of declining, so the row stands them on opposite edges of
// one line. The host is that line, the frame
// `worklist.dispositions.stories.tsx` gives the judgements sharing it.

const ITEM: WorklistItem = {
  id: "01a05500-0000-7000-8000-0000000000b1",
  source: "brief_item",
  category: "deals_at_risk",
  level: 3,
  consequence: "deal_drifts",
  title: "Advance the Northstar deal",
  band: "now",
  because: [],
  actions: ["act", "set_aside", "dismiss"],
  primary_action: "act",
  subject: { type: "deal", id: "01a05500-0000-7000-8000-00000000bb" },
};

const MARKED = { ...ITEM, actions: [] };
// Every verb answered at once — they go to different endpoints, so a story
// routing one would confirm nothing about the others.
function settleAtOnce() {
  installFetchStub({
    [`POST /brief/items/${ITEM.id}/act`]: () => jsonResponse(MARKED),
    [`POST /brief/items/${ITEM.id}/dismiss`]: () => jsonResponse(MARKED),
    [`POST /brief/items/${ITEM.id}/snooze`]: () => jsonResponse(MARKED),
    [`POST /brief/items/${ITEM.id}/unsnooze`]: () => jsonResponse(ITEM),
  });
}

// Never answers, the only way to draw the set mid-write.
function neverSettles() {
  const out = () => new Promise<Response>(() => {});
  installFetchStub({ [`POST /brief/items/${ITEM.id}/snooze`]: out });
}

// The hook is held by the LINE rather than by either component: all three verbs
// share one write, and two mutations would each carry their own settled flag, so
// acting and then dismissing would answer one item twice.
function Line({ item }: Readonly<{ item: WorklistItem }>) {
  const brief = useBriefAnswer(item);
  return (
    <div className="worklist-row-acts">
      <BriefSetAsides item={item} brief={brief} />
      {brief.offered("act") ? <BriefAct item={item} brief={brief} /> : null}
    </div>
  );
}

const meta: Meta<typeof BriefAct> = {
  title: "Records/Worklist/Brief verbs",
  component: BriefAct,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof BriefAct>;

// The press both interaction frames share: they differ in what the write does,
// not in the gesture, and a second copy is a second thing to fix.
const setAside: Story["play"] = async ({ canvasElement }) => {
  const canvas = within(canvasElement);
  await userEvent.click(await canvas.findByRole("button", { name: "Snooze" }));
};

function line(item: WorklistItem, stub: () => void) {
  return () => {
    stub();
    return (
      <StoryProviders>
        <ToastProvider>
          <Panel title="What to do next">
            <PanelBody>
              <Line item={item} />
            </PanelBody>
          </Panel>
          <ToastRegion />
        </ToastProvider>
      </StoryProviders>
    );
  };
}

/**
 * The overnight ranking's pick, and the two ways of declining it. Done is the
 * filled answer at the end of the line; Snooze and Dismiss are chosen INSTEAD of
 * it, and the distance between them is what says so.
 */
export const ActAndTheTwoWaysOfDeclining: Story = {
  render: line(ITEM, settleAtOnce),
};

/**
 * Each verb is drawn only where the SERVER offered it. The lane sends all three
 * today; a client that assumed so would keep drawing three the day one is
 * withheld, posting an answer nobody authorised.
 */
export const OnlyTheVerbsTheServerOffered: Story = {
  render: line({ ...ITEM, actions: ["act"] }, settleAtOnce),
};

/**
 * SET ASIDE UNTIL WHEN, and the way back from it. The toast names the hour the
 * item returns, in the reader's own zone, and carries the undo the snooze used
 * to lack — and the set stands down behind it, because an answered item is
 * patched in place rather than leaving the queue, so a second press is both
 * possible and wrong.
 */
export const SetAsideNamesWhenItReturns: Story = {
  render: line(ITEM, settleAtOnce),
  play: setAside,
};

/**
 * One verb pressed, the whole set standing down while the write is out. The verb
 * the reader PRESSED is the busy one — it keeps its focus and turns a mark, so
 * the wait is announced from where they are standing.
 */
export const AnAnswerBeingWritten: Story = {
  render: line(ITEM, neverSettles),
  play: setAside,
};
