/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { project360 } from "./projects.fixtures";
import { CommitmentsCard } from "./projectsections";

// The one card that names what a project owes, and its rows opened nothing.
// A reader who wanted the description, the assignee or the verbs behind a
// commitment had to find the same task on another screen to reach them.

const COMMITMENT = {
  activity_id: "a-1",
  subject: "Confirm the depot slot",
  due_at: "2026-09-11T09:00:00Z",
  assignee_id: null,
  assignee_name: null,
  overdue: false,
} satisfies components["schemas"]["Project360Commitment"];

// The shared builder, so the fixture is a COMPLETE Project360 rather than one
// asserted into the type: a cast fixture can drop a required field and still
// compile, and the test would go on passing after the wire shape moved.
function view(commitments: components["schemas"]["Project360Commitment"][]) {
  return project360({
    commitments: { data: commitments, page: { has_more: false } },
  });
}

function draw(onOpenTask?: (activityId: string) => void) {
  render(
    <LocaleProvider initial="en">
      <CommitmentsCard view={view([COMMITMENT])} onOpenTask={onOpenTask} />
    </LocaleProvider>,
  );
}

afterEach(cleanup);

describe("a project's commitments", () => {
  it("opens the task the row stands for", async () => {
    const opened: string[] = [];
    draw((activityId) => opened.push(activityId));

    await userEvent.click(
      screen.getByRole("button", { name: "Confirm the depot slot" }),
    );

    // The exact id, not merely that something was pressed: a row that opened
    // the wrong task would pass an assertion about the click alone.
    expect(opened).toEqual(["a-1"]);
  });

  it("stays flat where the screen mounts no task detail", () => {
    // The refusal case, and the one that says the button above is the
    // handler's doing rather than something the row draws regardless.
    draw();

    expect(screen.getByText("Confirm the depot slot")).toBeTruthy();
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("still says when a commitment is overdue", () => {
    // The row's other facts survive the subject becoming a control. A button
    // wrapping the whole row would have swallowed them.
    render(
      <LocaleProvider initial="en">
        <CommitmentsCard
          view={view([{ ...COMMITMENT, overdue: true }])}
          onOpenTask={vi.fn()}
        />
      </LocaleProvider>,
    );

    expect(screen.getByText(en["project.commitments.overdue"])).toBeTruthy();
  });
});
