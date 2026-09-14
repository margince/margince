// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ActionRow } from "./actionrow";
import { Button } from "./atoms";

const here = dirname(fileURLToPath(import.meta.url));

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

  // The half `Children.toArray` cannot answer.
  //
  // A child that is PRESENT and renders nothing counts as one element, so the
  // group is drawn — and `<DispositionVerbs/>` is exactly that on a row the
  // server offers no judgements for. What takes the line back then is
  // `.action-row-lead:empty` in the row's own sheet, which is asked after the
  // render; this holds the state that rule keys on.
  it("leaves the leading group empty when every secondary renders nothing", () => {
    function NoVerbsHere() {
      return null;
    }
    const { container } = render(
      <ActionRow primary={<Button variant="primary">Save changes</Button>}>
        <NoVerbsHere />
      </ActionRow>,
    );

    const lead = container.querySelector(".action-row-lead");
    expect(lead).not.toBeNull();
    // Nothing at all in it — not even whitespace, which would stop `:empty`
    // matching and leave the blank line the sheet is there to remove.
    expect(lead?.childNodes.length).toBe(0);
    expect(lead?.matches(":empty")).toBe(true);
  });
});

// The rule the case above leans on, read off the sheet rather than trusted.
//
// jsdom applies no stylesheet, so "the group disappears" cannot be asserted
// against the DOM: `:empty` matching proves the selector would hit, and this
// proves there is a selector to hit it. Both groups, because a row that took
// the blank line back on one edge and not the other is the same defect from
// the other side.
describe("actionrow.css takes the line back from an empty group", () => {
  it("hides a group whose children all rendered nothing", () => {
    const css = readFileSync(join(here, "actionrow.css"), "utf8");
    const rule = [...css.matchAll(/([^{}]*)\{([^}]*)\}/g)].find(
      ([, selector]) => selector.includes(":empty"),
    );
    expect(
      rule,
      "no :empty rule — a group whose children all rendered nothing keeps its flex line",
    ).not.toBeUndefined();
    const [, selector, body] = rule ?? ["", "", ""];
    expect(selector).toContain(".action-row-lead:empty");
    expect(selector).toContain(".action-row-trail:empty");
    // `display: none` and not a zero size: a flex item collects the row's gap
    // either way, so anything short of leaving the line keeps the air.
    expect(body).toMatch(/display:\s*none/);
  });
});
