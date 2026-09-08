// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

// One row of verbs, with one call to action at its end.
//
// The row used to draw its quiet verbs in a strip and then drop the lane's
// answer to a SECOND line under them, so a queue of ten rows spent ten extra
// lines on ten single buttons and the thing a reader came to press was the one
// verb not in the row of verbs. These assert the PLACEMENT, because the strip
// and the answer look almost identical in a screenshot when the row is wide
// enough to have room for both: what tells them apart is which group each verb
// is in, and nothing else on this page reads that.

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

describe("a row's verbs are one row with one call to action at its end", () => {
  it("holds the lane's answer on the trailing edge", async () => {
    oneRow(waitingRow());

    const reply = await screen.findByRole("button", {
      name: en["compose.reply"],
    });
    expect(reply.closest(".worklist-row-acts")).not.toBeNull();
    expect(reply.closest(".action-row-trail")).not.toBeNull();
  });

  // EVERY quieter verb, not a sample: the leading group is where a verb lands
  // when nobody placed it, so a test naming two of five would let the third be
  // promoted to the call to action with nothing failing.
  it("holds every quieter verb on the leading edge", async () => {
    oneRow(waitingRow());

    const open = await screen.findByRole("link", {
      name: en["worklist.verb.open"],
    });
    const quiet = [
      open,
      screen.getByRole("button", {
        name: en["worklist.disposition.verb.snooze"],
      }),
      screen.getByRole("button", {
        name: en["worklist.disposition.snoozeFor"],
      }),
      screen.getByRole("button", {
        name: en["worklist.disposition.verb.not_mine"],
      }),
      screen.getByRole("button", { name: en["worklist.verb.pin"] }),
    ];

    for (const verb of quiet) {
      expect(
        verb.closest(".action-row-lead"),
        `${verb.textContent || verb.getAttribute("aria-label")} is not in the leading group`,
      ).not.toBeNull();
    }
  });

  // The hand-off, on the one lane that carries one. Separate from the case above
  // because it is a task's verb and a waiting message has no assignee to move.
  it("holds the hand-off on the leading edge of a task's row", async () => {
    oneRow(row({ id: "task-1", source: "task", title: "Send the quote" }));

    const reassign = await screen.findByRole("button", {
      name: en["worklist.manager.reassign"],
    });
    expect(reassign.closest(".action-row-lead")).not.toBeNull();
  });

  // A lane whose verbs are EQUAL has no call to action, and drawing one would
  // be the product claiming an expectation it has not got: held, no-show and
  // cancelled are three records of what already happened.
  it("gives a lane of equal verbs no trailing group at all", async () => {
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
    const acts = held.closest(".worklist-row-acts");
    expect(held.closest(".action-row-lead")).not.toBeNull();
    for (const verb of ["meetingNoShow", "meetingCanceled"] as const) {
      expect(
        screen
          .getByRole("button", { name: en[`worklist.verb.${verb}`] })
          .closest(".action-row-lead"),
      ).not.toBeNull();
    }
    expect(acts?.querySelector(".action-row-trail")).toBeNull();
  });
});
