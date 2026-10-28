// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

// One right-aligned line of verbs: Done, the lane's answer, then the row's menu.
// The menu closes every row's line, so the answer lands at one x down the queue.
// These assert the order, because on a lane whose answer is a ghost button the
// order alone says which verb is the answer.

import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import {
  day,
  findRowMenu,
  renderWorklist,
  row,
  stub,
  type WorklistItem,
} from "../../testing/worklist.testkit";

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
// the answer, Done stands before it, and the rest are lines of the menu.
const WAITING = "Re: pricing for the retrofit";
function waitingRow(): WorklistItem {
  return row({
    id: "01a05500-0000-7000-8000-0000000000a1",
    source: "customer_waiting",
    category: "customer_waiting",
    level: 1,
    consequence: "buyer_waits",
    title: WAITING,
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

// The lines of the row's menu, in order, read off the panel the trigger opens.
// The panel is portalled to the body, so it is reached through the trigger.
async function menuLines(): Promise<string[]> {
  const trigger = await findRowMenu();
  await userEvent.setup().click(trigger);
  const panel = document.getElementById(
    trigger.getAttribute("aria-controls") ?? "",
  );
  expect(panel, "the menu opened no panel").not.toBeNull();
  return [...(panel?.querySelectorAll("button, a") ?? [])].map(
    (line) => line.textContent ?? "",
  );
}

describe("a row's verbs are one line with the row's menu last", () => {
  it("ends the line with the menu, the lane's answer just before it", async () => {
    oneRow(waitingRow());

    const reply = await screen.findByRole("button", {
      name: en["compose.reply"],
    });
    const drawn = verbsInOrder();
    expect(drawn.at(-1)).toBe(await findRowMenu());
    expect(drawn.at(-2)).toBe(reply);
  });

  // The whole line in order, every control named, because the order is the
  // layout. A test naming two of three would let a verb pass the answer with
  // nothing failing. The Brief's card reads in the same order.
  it("draws Done, then the answer, then the menu", async () => {
    oneRow(waitingRow());

    const reply = await screen.findByRole("button", {
      name: en["compose.reply"],
    });
    expect(verbsInOrder()).toEqual([
      screen.getByRole("button", {
        name: en["worklist.disposition.verb.handled"],
      }),
      reply,
      await findRowMenu(),
    ]);
  });

  // The menu reads most often first: the snooze, then the reader's own mark,
  // then the judgement that takes the row off the list. The row's title links
  // its record, so the menu offers no second way there.
  it("puts the snooze first in the menu and the hand-backs last", async () => {
    oneRow(waitingRow());
    await screen.findByRole("button", { name: en["compose.reply"] });

    const lines = await menuLines();

    expect(lines[0]).toBe(en["worklist.disposition.verb.snooze"]);
    expect(lines.at(-1)).toBe(en["worklist.disposition.verb.not_mine"]);
    expect(lines.at(-2)).toBe(en["worklist.verb.pin"]);
    expect(lines).not.toContain(en["worklist.verb.open"]);
  });

  // The hand-off, on the one lane that carries one: a task's verb, since a
  // waiting message has no assignee to move. It follows the pin, the other
  // mark a reader puts on the row, and precedes the hand-backs.
  it("puts the hand-off between the pin and the hand-backs", async () => {
    oneRow(
      row({
        id: "task-1",
        source: "task",
        title: "Send the quote",
        actions: ["complete"],
        dispositions: ["not_mine"],
      }),
    );
    await screen.findByText("Send the quote");

    expect(await menuLines()).toEqual([
      en["worklist.verb.pin"],
      en["worklist.manager.reassign"],
      en["worklist.disposition.verb.not_mine"],
    ]);
  });

  // Finishing a task is the day's next move, so its answer wears the fill and
  // no other verb on the line does. Asserted over every filled control, since a
  // second fill would be two calls to action.
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
    expect(drawn.at(-2)).toBe(done);
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
  // Asserted over the addresses on the line and in its menu, so it holds when a
  // verb joins the ask or its wording changes. The ask has no answer of its
  // own, so the one link left stands on the line.
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
    expect(await menuLines()).not.toContain(en["worklist.verb.open"]);
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
        actions: ["decide", "cancel"],
      }),
    );

    const update = await screen.findByRole("button", {
      name: en["worklist.verb.meetingUpdateFor"].replace(
        "{subject}",
        "Quarterly review with Turbinenbau",
      ),
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
        name: en["worklist.verb.meetingCanceledFor"].replace(
          "{subject}",
          "Quarterly review with Turbinenbau",
        ),
      }),
    );
    expect(
      cancelled,
      "cancelled is out of the lane's own order",
    ).toBeGreaterThan(at);
  });

  it("draws one control per destination, not one per verb", async () => {
    stub(
      day({
        queue: [
          row({
            title: "Send the proposal",
            subject: {
              type: "contact",
              id: "01a05500-0000-7000-8000-000000000009",
              label: "Anna Weber",
            },
            // Both verbs open the same record.
            actions: ["complete", "snooze"],
          }),
        ],
        summary: { urgent: 0, due: 1, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    // The title opens the task; one separate control reaches its contact.
    await screen.findByText(/Send the proposal/);
    await userEvent.setup().click(await findRowMenu());
    const verbsToRecord = [
      ...document.querySelectorAll(
        ".worklist-row-acts a, .overflow-menu-items a",
      ),
    ].filter((link) =>
      link
        .getAttribute("href")
        ?.includes("01a05500-0000-7000-8000-000000000009"),
    );
    expect(verbsToRecord).toHaveLength(1);
    expect(
      screen.getByRole("button", { name: /^Send the proposal/ }),
    ).toBeTruthy();
  });

  it("offers a verb that goes somewhere, and none that does not", async () => {
    stub(
      day({
        queue: [
          row({
            id: "with-record",
            title: "Anna Weber",
            source: "customer_waiting",
            category: "customer_waiting",
            consequence: "buyer_waits",
            subject: {
              type: "contact",
              id: "01a05500-0000-7000-8000-000000000001",
            },
            actions: ["open"],
          }),
          row({
            id: "no-destination",
            title: "Add contact from mail",
            source: "approval",
            category: "decisions",
            consequence: "data_drifts",
            // `decide` is answered on this page, so it has nowhere to link.
            actions: ["decide"],
          }),
        ],
        summary: { urgent: 1, due: 0, lower_priority: 1, total: 2 },
      }),
    );
    renderWorklist();

    // The row that names a record links it, from its title. The row whose
    // verb has nowhere to go links nothing.
    const record = await screen.findByRole("link", { name: /Anna Weber/ });
    expect(record.getAttribute("href")).toContain(
      "01a05500-0000-7000-8000-000000000001",
    );
    expect(
      screen.queryByRole("link", { name: /Add contact from mail/ }),
    ).toBeNull();
  });
});
