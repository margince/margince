/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../../i18n";
import { type Grounding, VerdictHead } from "./verdict";

// The call is the one thing a scanner reads, and the working behind it is the
// thing a doubter reads. The disclosure is what keeps those two readers from
// having to share one block.

const GROUNDS: Grounding[] = [
  { key: "relationship", quote: "No reply in 18 days", from: "Relationship" },
  { key: "payment", quote: "Pays 9 days after due", from: "Payment" },
];

function renderHead(restsOn?: Grounding[]) {
  render(
    <LocaleProvider initial="en">
      <VerdictHead
        label="At risk"
        tone="danger"
        because="Waiting on them"
        restsOn={restsOn}
      />
    </LocaleProvider>,
  );
}

afterEach(() => {
  cleanup();
});

describe("VerdictHead grounding", () => {
  it("keeps the working shut but says how much of it there is", () => {
    renderHead(GROUNDS);
    // Shut: the quotes are not merely hidden from view, they are not rendered,
    // so a screen reader walking the head meets the call and not the working.
    expect(screen.queryByText("No reply in 18 days")).toBeNull();
    const toggle = screen.getByRole("button", { name: /What this rests on/ });
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    // The count is what makes a shut block judgeable rather than a mystery.
    expect(toggle.textContent).toContain("2");
  });

  it("opens the working on a click, quote beside source", async () => {
    renderHead(GROUNDS);
    await userEvent.click(
      screen.getByRole("button", { name: /What this rests on/ }),
    );
    expect(screen.getByText("No reply in 18 days")).toBeTruthy();
    expect(screen.getByText("Relationship")).toBeTruthy();
    expect(screen.getByText("Pays 9 days after due")).toBeTruthy();
  });

  it("offers no disclosure when the call rests on nothing showable", () => {
    // Different from a call resting on nothing: the head still states the
    // call, and an empty trigger reading "0" would invite a click onto a
    // block with nothing in it.
    renderHead([]);
    expect(screen.queryByRole("button", { name: /What this rests on/ })).toBe(
      null,
    );
    expect(screen.getByText("At risk")).toBeTruthy();
  });
});
