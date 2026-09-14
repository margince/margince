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
// the answer, the record link and three judgements are the quieter half, and the
// pin and the hand-off are glyphs.
function waitingRow(): WorklistItem {
  return row({
    id: "01a05500-0000-7000-8000-0000000000a1",
    source: "customer_waiting",
    category: "customer_waiting",
    level: 1,
    consequence: "buyer_waits",
    title: "Re: pricing for the retrofit",
    actions: ["open", "reply"],
    dispositions: ["snooze", "not_mine"],
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

  // THE WHOLE LINE, in order, and every verb on it named — not a sample. The
  // order IS the layout here: the ways to the record come first, the reader's
  // own GLYPH sits among the labelled verbs, the ways to put the row down
  // follow, and the answer is the tail. A test naming two of six would let a
  // third be promoted past the call to action, or let the glyph slide out to
  // the end of the line, with nothing failing.
  //
  // The pin is among the WORDS on purpose: a lone 32px glyph closing a line has
  // no label to read as a verb, so it reads as a stray mark.
  it("draws the quieter verbs before it, glyph among the words", async () => {
    oneRow(waitingRow());

    const reply = await screen.findByRole("button", {
      name: en["compose.reply"],
    });
    const inOrder = [
      screen.getByRole("link", { name: en["worklist.verb.open"] }),
      screen.getByRole("button", { name: en["worklist.verb.pin"] }),
      screen.getByRole("button", {
        name: en["worklist.disposition.verb.snooze"],
      }),
      screen.getByRole("button", {
        name: en["worklist.disposition.snoozeFor"],
      }),
      screen.getByRole("button", {
        name: en["worklist.disposition.verb.not_mine"],
      }),
      reply,
    ];

    expect(verbsInOrder()).toEqual(inOrder);
  });

  // The hand-off, on the one lane that carries one. Separate from the case above
  // because it is a task's verb and a waiting message has no assignee to move.
  //
  // It is a glyph too, so it stands beside the pin and for the same reason:
  // the two controls with no word on them stay together among the labelled
  // ones, where a reader meets a name beside them.
  it("draws the hand-off beside the pin, among the words", async () => {
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
    expect(drawn.indexOf(reassign)).toBeGreaterThan(
      drawn.indexOf(
        screen.getByRole("button", { name: en["worklist.verb.pin"] }),
      ),
    );
    expect(drawn.indexOf(reassign)).toBeLessThan(
      drawn.indexOf(
        screen.getByRole("button", {
          name: en["worklist.disposition.verb.not_mine"],
        }),
      ),
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
  // be the product claiming an expectation it has not got: held, no-show and
  // cancelled are three records of what already happened. Nothing on the line
  // is filled, and the three keep the order the lane drew them in.
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

    const held = await screen.findByRole("button", {
      name: en["worklist.verb.meetingHeld"],
    });
    const drawn = verbsInOrder();
    expect(filled(drawn)).toEqual([]);
    let at = drawn.indexOf(held);
    expect(
      at,
      "the outcomes are not on the row's line of verbs",
    ).toBeGreaterThan(-1);
    for (const verb of ["meetingNoShow", "meetingCanceled"] as const) {
      const next = drawn.indexOf(
        screen.getByRole("button", { name: en[`worklist.verb.${verb}`] }),
      );
      expect(next, `${verb} is out of the lane's own order`).toBeGreaterThan(
        at,
      );
      at = next;
    }
  });
});
