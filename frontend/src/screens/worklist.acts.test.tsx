// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

// One row of verbs, with the lane's answer at its head.
//
// The row drew its verbs in two groups — the quiet ones on the leading edge and
// the answer held on the trailing one — which reads well on a row wide enough
// for the whole line and breaks on every row that is not: once the line
// wrapped, the trailing group dropped alone to a second line, so a queue of ten
// rows drew ten one-button lines and the thing a reader came to press was the
// one control not in the row of controls.
//
// These assert the ORDER, because order is now the whole of what says which
// verb is the answer on a lane whose answer is a ghost — a filled Done and a
// ghost Reply are both their lane's call to action, and a screenshot of either
// looks like a row of verbs. Nothing else on this page reads it.

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
// line rather than off the page: every assertion below is about which control
// comes first in it, and a page-wide query would answer with whatever the
// screen drew above the queue.
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

describe("a row's verbs are one line with the lane's answer at its head", () => {
  it("leads the line with the lane's answer", async () => {
    oneRow(waitingRow());

    const reply = await screen.findByRole("button", {
      name: en["compose.reply"],
    });
    expect(verbsInOrder()[0]).toBe(reply);
  });

  // THE WHOLE LINE, in order, and every verb on it named — not a sample. The
  // order IS the layout here: the answer leads, the ways to the record follow
  // it, the reader's own GLYPH sits among the labelled verbs, and the ways to
  // put the row down are the tail. A test naming two of six would let the
  // third be promoted ahead of the call to action, or let the glyph slide back
  // to the end, with nothing failing.
  //
  // The pin goes before the judgements because the END of this line is where it
  // wraps: at the tail it went over alone, and a lone 32px glyph on a line of
  // its own reads as a stray mark rather than as a verb.
  it("draws the quieter verbs after it, glyph before the tail", async () => {
    oneRow(waitingRow());

    const reply = await screen.findByRole("button", {
      name: en["compose.reply"],
    });
    const inOrder = [
      reply,
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
    ];

    expect(verbsInOrder()).toEqual(inOrder);
  });

  // The hand-off, on the one lane that carries one. Separate from the case above
  // because it is a task's verb and a waiting message has no assignee to move.
  //
  // It is a glyph too, so it stands where the pin does — before the judgements
  // — and for the same reason: a task row carries as many verbs as a waiting
  // one, so the tail of its line has to be a word as well.
  it("draws the hand-off beside the pin, before the tail", async () => {
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

  // The lane whose answer is FILLED as well as first: finishing a task is the
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
    expect(drawn[0]).toBe(done);
    expect(filled(drawn)).toEqual([done]);
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
