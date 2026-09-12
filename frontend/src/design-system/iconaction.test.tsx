// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { IconAction } from "./iconaction";

// A glyph verb's second sentence.
//
// `label` answers "what is this control" and stops there. Some verbs need more
// — a pin is a personal ordering preference that holds until it is undone, and
// none of that is in the word "Pin". `hint` carries it.
//
// The split is the whole point: the hint is a DESCRIPTION, never part of the
// NAME. A control list that read the sentence once per row would be worse than
// the bare word, so a screen reader must still hear "Pin" as the name.

afterEach(cleanup);

function describedTextOf(control: HTMLElement): string {
  return (control.getAttribute("aria-describedby") ?? "")
    .split(/\s+/)
    .map((id) => document.getElementById(id)?.textContent ?? "")
    .join(" ")
    .trim();
}

describe("a glyph verb that needs a second sentence", () => {
  it("keeps the hint out of the name and in the description", () => {
    render(
      <IconAction
        label="Pin"
        hint="Only you see it."
        icon={<span aria-hidden="true">*</span>}
      />,
    );

    // The NAME is the bare verb: this is what a control list reads.
    const control = screen.getByRole("button", { name: "Pin" });
    expect(describedTextOf(control)).toBe("Only you see it.");
  });

  // A caller that passes none is exactly the control it was — nineteen callers
  // do, and none of them should gain a description they never asked for.
  it("describes nothing where the caller gave no hint", () => {
    render(<IconAction label="Pin" icon={<span aria-hidden="true">*</span>} />);

    const control = screen.getByRole("button", { name: "Pin" });
    expect(control.getAttribute("aria-describedby")).toBeNull();
  });
});
