// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

// One right-aligned row of verbs, with the lane's answer LAST.
//
// A reader runs a row left to right — the rank, the kind, the work — and the
// verb the lane is asking for is the end of that sentence. Held on the trailing
// edge it also lands at one x down the whole queue however many quiet verbs the
// row ahead of it carries, so a rep answering row after row presses in the same
// place every time.
//
// These assert the ORDER, because order is the whole of what says which verb is
// the answer on a lane whose answer is a ghost — a filled Done and a ghost
// Reply are both their lane's call to action, and a screenshot of either looks
// like a row of verbs. Nothing else on this page reads it.

import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import {
  day,
  renderWorklist,
  row,
  stub,
  type WorklistItem,
} from "./worklist.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function oneRow(item: WorklistItem) {
  stub(
    day({
      queue: [item],
      summary: { urgent: 1, due: 0, lower_priority: 0, total: 1 },
    }),
  );
  renderWorklist();
}

// A buyer waiting on an answer: the lane with the most verbs on it. The reply is
// the answer, the record link, Done and the menu of set-asides are the quieter
// half, and the pin is a glyph.
function waitingRow(): WorklistItem {
  return row({
    id: "01a05500-0000-7000-8000-0000000000a1",
    source: "customer_waiting",
    category: "customer_waiting",
    level: 1,
    consequence: "buyer_waits",
    title: "Re: pricing for the retrofit",
    actions: ["open", "reply"],
    dispositions: ["handled", "snooze", "not_mine"],
    subject: {
      type: "deal",
      id: "01a05500-0000-7000-8000-00000000bbbb",
      label: "Acme Expansion",
    },
  });
}

// The verbs of one row, in the order they are drawn. Read off the row's own
// line rather than off the page: every assertion below is about WHERE in it a
// control stands, and a page-wide query would answer with whatever the screen
// drew above the queue.
function verbsInOrder(): HTMLElement[] {
  const acts = document.querySelector(".worklist-row-acts");
  expect(acts, "the row drew no line of verbs").not.toBeNull();
  return [...(acts?.querySelectorAll("button, a") ?? [])].filter(
    (node): node is HTMLElement => node instanceof HTMLElement,
  );
}

/** The filled controls on the row's line — the lane's answer, or nothing. */
function filled(verbs: readonly HTMLElement[]): HTMLElement[] {
  return verbs.filter((verb) => verb.classList.contains("btn-primary"));
}

describe("a row's verbs are one line with the lane's answer last", () => {
  it("ends the line with the lane's answer", async () => {
    oneRow(waitingRow());

    const reply = await screen.findByRole("button", {
      name: en["compose.reply"],
    });
    expect(verbsInOrder().at(-1)).toBe(reply);
  });

  // The whole line in order, every verb named, because the order is the
  // layout. A test naming two of five would let a verb pass the answer with
  // nothing failing. The Brief's card reads in the same order, so a row's
  // answer is at one place on both surfaces.
  it("draws the way in, the glyphs with the menu, Done, then the answer", async () => {
    oneRow(waitingRow());

    const reply = await screen.findByRole("button", {
      name: en["compose.reply"],
    });
    const inOrder = [
      screen.getByRole("link", { name: en["worklist.verb.open"] }),
      screen.getByRole("button", { name: en["worklist.verb.pin"] }),
      screen.getByRole("button", { name: en["worklist.disposition.menu"] }),
      screen.getByRole("button", {
        name: en["worklist.disposition.verb.handled"],
      }),
      reply,
    ];

    expect(verbsInOrder()).toEqual(inOrder);
  });

  // The hand-off, on the one lane that carries one. Separate from the case above
  // because it is a task's verb and a waiting message has no assignee to move.
  //
  // It is a glyph too, so it stands between the pin and the menu. The square
  // controls stay together, with a labelled verb on either side.
  it("draws the hand-off between the pin and the menu", async () => {
    oneRow(
      row({
        id: "task-1",
        source: "task",
        title: "Send the quote",
        dispositions: ["not_mine"],
      }),
    );

    const reassign = await screen.findByRole("button", {
      name: en["worklist.manager.reassign"],
    });
    const drawn = verbsInOrder();
    const at = drawn.indexOf(reassign);
    expect(drawn[at - 1]).toBe(
      screen.getByRole("button", { name: en["worklist.verb.pin"] }),
    );
    expect(drawn[at + 1]).toBe(
      screen.getByRole("button", { name: en["worklist.disposition.menu"] }),
    );
  });

  // The lane whose answer is FILLED as well as last: finishing a task is the
  // day's next move, so it wears the fill and every quiet verb beside it does
  // not. Asserted as the WHOLE set of filled controls, because a second fill on
  // the line is two calls to action, which is none.
  it("fills the answer and nothing else on the line", async () => {
    oneRow(
      row({
        id: "task-2",
        source: "task",
        title: "Send the quote",
        actions: ["complete"],
      }),
    );

    const done = await screen.findByRole("button", {
      name: en["tasks.complete"],
    });
    const drawn = verbsInOrder();
    expect(drawn.at(-1)).toBe(done);
    expect(filled(drawn)).toEqual([done]);
  });

  // ONE CONTROL PER DESTINATION, whatever the WORDS on the controls.
  //
  // An introduction ask carries `decide` and `open`, and both of them reach the
  // colleague's Network tab — the ask is answered there and nowhere else. Two
  // links to one page ask the reader to choose between the same thing twice, and
  // because the two verbs are different words, a rule that told controls apart
  // by their label would offer both.
  //
  // Asserted over the ADDRESSES on the line rather than by naming the survivor:
  // what has to hold is that no address is offered twice, which stays true when
  // a verb joins the ask or its wording changes.
  it("offers one control per destination, whatever the words", async () => {
    oneRow(
      row({
        id: "01a05500-0000-7000-8000-0000000000i1",
        source: "introduction_request",
        category: "decisions",
        level: 5,
        consequence: "work_blocked",
        title: "Katrin asked for an introduction",
        actions: ["decide", "open"],
        subject: {
          type: "contact",
          id: "01a05500-0000-7000-8000-0000000000dd",
          label: "Dana Buyer",
        },
      }),
    );

    const decide = await screen.findByRole("link", {
      name: en["worklist.verb.decide"],
    });
    const addresses = verbsInOrder()
      .filter(
        (verb): verb is HTMLAnchorElement => verb instanceof HTMLAnchorElement,
      )
      .map((link) => link.getAttribute("href"));
    expect(addresses).toEqual([decide.getAttribute("href")]);
  });

  // A lane whose verbs are EQUAL has no call to action, and drawing one would
  // be the product claiming an expectation it has not got: saying what came of
  // a meeting and recording that it was called off are two answers of equal
  // standing. Nothing on the line is filled, and the two keep the order the
  // lane drew them in.
  it("promotes none of a lane's equal verbs", async () => {
    oneRow(
      row({
        id: "01a05500-0000-7000-8000-0000000000m1",
        source: "meeting_outcome",
        category: "meetings",
        title: "Quarterly review with Turbinenbau",
        occurred_at: "2026-08-31T08:00:00Z",
        actions: ["decide"],
      }),
    );

    const update = await screen.findByRole("button", {
      name: en["worklist.verb.meetingUpdate"],
    });
    const drawn = verbsInOrder();
    expect(filled(drawn)).toEqual([]);
    const at = drawn.indexOf(update);
    expect(
      at,
      "the outcomes are not on the row's line of verbs",
    ).toBeGreaterThan(-1);
    const cancelled = drawn.indexOf(
      screen.getByRole("button", {
        name: en["worklist.verb.meetingCanceled"],
      }),
    );
    expect(
      cancelled,
      "cancelled is out of the lane's own order",
    ).toBeGreaterThan(at);
  });
});
