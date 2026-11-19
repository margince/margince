import { describe, expect, it } from "vitest";
import { withPrimaryDefault, withPrimaryMarked } from "./createrows";

describe("withPrimaryMarked", () => {
  it("makes one row primary across every type", () => {
    const rows = [
      { email: "a@x.test", email_type: "work", is_primary: "true" },
      { email: "b@x.test", email_type: "other", is_primary: "" },
      { email: "c@x.test", email_type: "personal", is_primary: "true" },
    ];
    const marked = withPrimaryMarked(rows, 1, "is_primary");
    expect(marked.map((row) => row.is_primary)).toEqual(["", "true", ""]);
  });
});

describe("withPrimaryDefault", () => {
  it("marks the first row when none is primary", () => {
    const sent = withPrimaryDefault([
      { email: "a@x.test", is_primary: false },
      { email: "b@x.test", is_primary: false },
    ]);
    expect(sent.map((row) => row.is_primary)).toEqual([true, false]);
  });

  it("leaves a chosen primary and an empty list alone", () => {
    const chosen = [
      { email: "a@x.test", is_primary: false },
      { email: "b@x.test", is_primary: true },
    ];
    expect(withPrimaryDefault(chosen)).toBe(chosen);
    expect(withPrimaryDefault([])).toEqual([]);
  });
});
