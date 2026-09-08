// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ActionRow } from "./actionrow";
import { Button } from "./atoms";

// What the row DRAWS, as opposed to what `actionrow.test.ts` reads out of the
// tree's stylesheets: that gate asks whether a row of buttons gets its gap, and
// a group holding nothing is a group it has no reason to look at.

afterEach(cleanup);

describe("the row's two groups", () => {
  it("draws no leading group for a row that has only a call to action", () => {
    const { container } = render(
      <ActionRow primary={<Button variant="primary">Save changes</Button>} />,
    );
    // An empty lead is still a flex item on a row that WRAPS: at a width that
    // fits the primary but not the primary plus `--gapActions`, it takes the
    // first line and leaves the one button under a line of air holding nothing.
    expect(container.querySelector(".action-row-lead")).toBeNull();
    expect(container.querySelector(".action-row-trail")).not.toBeNull();
  });

  it("draws no leading group for secondaries a condition ruled out", () => {
    const canDiscard = false;
    const { container } = render(
      <ActionRow primary={<Button variant="primary">Save changes</Button>}>
        {canDiscard && <Button>Discard</Button>}
      </ActionRow>,
    );
    // `Children.count` would see one child here and draw the empty group,
    // which is why the check is over `Children.toArray`.
    expect(container.querySelector(".action-row-lead")).toBeNull();
  });

  it("draws both groups when the verbs actually divide", () => {
    const { container } = render(
      <ActionRow primary={<Button variant="primary">Accept</Button>}>
        <Button>Reject</Button>
      </ActionRow>,
    );
    expect(container.querySelector(".action-row-lead")).not.toBeNull();
    expect(container.querySelector(".action-row-trail")).not.toBeNull();
  });

  it("draws no trailing group for a row of secondaries alone", () => {
    const { container } = render(
      <ActionRow>
        <Button>Preview</Button>
        <Button>Duplicate</Button>
      </ActionRow>,
    );
    expect(container.querySelector(".action-row-trail")).toBeNull();
    expect(container.querySelector(".action-row-lead")).not.toBeNull();
  });
});
