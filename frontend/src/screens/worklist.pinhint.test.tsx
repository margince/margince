// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { day, renderWorklist, row, stub } from "./worklist.testkit";

// The pin says what it does.
//
// The control is a glyph and its whole vocabulary was "Pin" / "Unpin". A reader
// could not tell from it whether pinning marks the row urgent, whether a
// colleague sees it, or how long it lasts — and the answers matter: it is a
// personal ordering preference, private to one reader, that holds until undone.
//
// The row has no space for a sentence, so it is the control's DESCRIPTION,
// reached by hover, by focus and by a screen reader.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the pin verb", () => {
  it("explains what pinning does without renaming the control", async () => {
    stub(
      day({
        queue: [row({ id: "t-1", title: "Call the buyer back" })],
        summary: { urgent: 0, due: 1, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    // Still named by the bare verb: the sentence must not reach the NAME, or a
    // queue of thirty rows repeats it thirty times in a control list.
    const pin = await screen.findByRole("button", {
      name: en["worklist.verb.pin"],
    });

    const described = (pin.getAttribute("aria-describedby") ?? "")
      .split(/\s+/)
      .map((id) => document.getElementById(id)?.textContent ?? "")
      .join(" ");
    expect(described).toContain(en["worklist.verb.pinHint"]);
  });

  // THE OTHER STATE, which is where a shared sentence goes wrong. One hint for
  // both had the Unpin button describing itself as keeping the row on top —
  // the control saying the opposite of what pressing it now does.
  it("describes removing the pin once the row is pinned", async () => {
    stub(
      day({
        queue: [
          row({
            id: "t-1",
            title: "Call the buyer back",
            because: [{ kind: "pinned" }],
          }),
        ],
        summary: { urgent: 0, due: 1, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    const unpin = await screen.findByRole("button", {
      name: en["worklist.verb.unpin"],
    });

    const described = (unpin.getAttribute("aria-describedby") ?? "")
      .split(/\s+/)
      .map((id) => document.getElementById(id)?.textContent ?? "")
      .join(" ");
    expect(described).toContain(en["worklist.verb.unpinHint"]);
    // And not the pin's own sentence, which is the defect this holds.
    expect(described).not.toContain(en["worklist.verb.pinHint"]);
  });
});
