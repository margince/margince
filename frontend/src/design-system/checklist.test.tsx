/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { Badge, Button } from "./atoms";
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

  it("seats a grey badge, not a verb, on a waiting step, in the caller's words when given", () => {
    render(
      <Checklist>
        <ChecklistStep state="waiting" title="Turn on bookings" />
        <ChecklistStep
          state="waiting"
          waitingLabel="Invited"
          title="Your teammate joins"
        />
      </Checklist>,
    );
    const [plain, worded] = screen.getAllByRole("listitem");
    const badge = plain.querySelector(".checklist-action .badge");
    expect(badge?.textContent).toBe("Waiting");
    expect(badge?.className).toBe("badge");
    expect(worded.querySelector(".checklist-action .badge")?.textContent).toBe(
      "Invited",
    );
    expect(within(plain).queryByRole("button")).toBeNull();
    expect(plain.getAttribute("aria-current")).toBeNull();
    // The badge already says it; the state is not read a second time.
    expect(plain.textContent?.match(/Waiting/g)).toHaveLength(1);
  });

  it("refuses an action on a waiting step at the type level", () => {
    render(
      <Checklist>
        {/* @ts-expect-error a waiting step has nothing the reader can do */}
        <ChecklistStep state="waiting" title="Bookings" action={<b>Go</b>} />
      </Checklist>,
    );
    expect(screen.queryByText("Go")).toBeNull();
  });

  it("captions the list with its count and ties the caption to it", () => {
    render(
      <Checklist count={{ done: 2, total: 4 }}>
        <ChecklistStep state="done" title="Connect your mail" />
      </Checklist>,
    );
    const list = screen.getByRole("list");
    const captionId = list.getAttribute("aria-describedby");
    expect(captionId).not.toBeNull();
    expect(document.getElementById(captionId ?? "")?.textContent).toBe(
      "2 of 4 done",
    );
  });

  it("draws the inline trail as marks and titles only, named by its label", () => {
    render(
      <Checklist variant="inline" label="Steps">
        <ChecklistStep
          state="done"
          title="Account"
          caption="Gmail"
          action={<Badge tone="success">Done</Badge>}
        />
        <ChecklistStep state="current" title="What to bring in">
          <p>A body the trail has no room for.</p>
        </ChecklistStep>
        <ChecklistStep state="open" title="Sign in" />
      </Checklist>,
    );
    const list = screen.getByRole("list", { name: "Steps" });
    expect(list.className).toContain("checklist-inline");
    expect(list.querySelector(".checklist-action")).toBeNull();
    expect(list.querySelector(".t-caption")).toBeNull();
    expect(screen.queryByText("A body the trail has no room for.")).toBeNull();
    const [done, current] = within(list).getAllByRole("listitem");
    expect(done.textContent).toContain("Done");
    expect(current.getAttribute("aria-current")).toBe("step");
  });
});
