// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { keepUnsubmitted } from "./create";

// A quick-capture form stays open to take the next record, and nothing stops
// the reader typing while the previous save is still in flight. What lands
// then decides whether they lose that typing, so the rule is proven here
// rather than left to the shape of the render.
describe("what survives a save on a form that stays open", () => {
  const defaults = { kind: "work" };

  it("clears a field still holding exactly what was sent", () => {
    const current = { full_name: "Dana Quick", kind: "work" };
    expect(keepUnsubmitted(current, current, defaults)).toEqual(defaults);
  });

  it("keeps a field the reader changed while the save was in flight", () => {
    const submitted = { full_name: "Dana Quick", title: "VP Finance" };
    const current = { full_name: "Sam Next", title: "VP Finance" };
    // The name is the NEXT person's and stays; the title still belongs to the
    // record that just saved and goes.
    expect(keepUnsubmitted(current, submitted, defaults)).toEqual({
      kind: "work",
      full_name: "Sam Next",
    });
  });

  it("keeps a field that had no submitted counterpart at all", () => {
    // Typed into an empty box after the submit snapshot was taken.
    const submitted = { full_name: "Dana Quick" };
    const current = { full_name: "Dana Quick", email: "sam@next.test" };
    expect(keepUnsubmitted(current, submitted, defaults)).toEqual({
      kind: "work",
      email: "sam@next.test",
    });
  });

  it("never carries an empty value forward as if it were typed", () => {
    const submitted = { full_name: "Dana Quick", title: "" };
    const current = { full_name: "Dana Quick", title: "" };
    expect(keepUnsubmitted(current, submitted, defaults)).toEqual(defaults);
  });

  it("lets a kept value outrank the default for its own field", () => {
    const submitted = { kind: "work" };
    const current = { kind: "mobile" };
    expect(keepUnsubmitted(current, submitted, defaults)).toEqual({
      kind: "mobile",
    });
  });
});
