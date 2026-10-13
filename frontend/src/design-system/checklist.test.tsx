/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { Button } from "./atoms";
import { Checklist, ChecklistStep } from "./checklist";

afterEach(cleanup);

function Setup() {
  return (
    <Checklist>
      <ChecklistStep
        state="done"
        title="Connect your mail"
        caption="admin@demo.test"
      />
      <ChecklistStep
        state="current"
        title="Connect your calendar"
        action={<Button variant="primary">Connect calendar</Button>}
      />
      <ChecklistStep state="open" title="Add a booking link">
        <p>Your link already exists.</p>
      </ChecklistStep>
    </Checklist>
  );
}

describe("Checklist", () => {
  it("is an ordered list of the steps, in source order", () => {
    render(<Setup />);
    const list = screen.getByRole("list");
    expect(list.tagName).toBe("OL");
    const titles = within(list)
      .getAllByRole("listitem")
      .map(
        (item) =>
          item.querySelector(".checklist-title")?.firstChild?.textContent,
      );
    expect(titles).toEqual([
      "Connect your mail",
      "Connect your calendar",
      "Add a booking link",
    ]);
  });

  it("marks the current step, and only that one, as the step", () => {
    render(<Setup />);
    const current = screen
      .getAllByRole("listitem")
      .filter((item) => item.getAttribute("aria-current") === "step");
    expect(current).toHaveLength(1);
    expect(current[0].textContent).toContain("Connect your calendar");
  });

  it("says each step's state in words, not only in its mark", () => {
    render(<Setup />);
    const [done, current, open] = screen.getAllByRole("listitem");
    expect(done.textContent).toContain("Done");
    expect(current.textContent).toContain("Still to do");
    expect(open.textContent).toContain("Still to do");
    expect(done.textContent).not.toContain("Still to do");
  });

  it("seats the action in the step's trailing cell and the body under the title", () => {
    render(<Setup />);
    const [, current, open] = screen.getAllByRole("listitem");
    const button = within(current).getByRole("button", {
      name: "Connect calendar",
    });
    expect(button.closest(".checklist-action")).not.toBeNull();
    const body = within(open).getByText("Your link already exists.");
    expect(body.closest(".checklist-text")).not.toBeNull();
  });
});
