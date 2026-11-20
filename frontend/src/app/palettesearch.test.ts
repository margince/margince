import { describe, expect, it } from "vitest";
import { mailLeftOut } from "./palettesearch";

describe("mailLeftOut", () => {
  it.each([
    ["co", true],
    ["con", true],
    ["cont", false],
    ["acme con", true],
    ["con acme", false],
    ["übe", true],
    ["über", false],
    ["übe", true],
    ["việt", false],
    ["con ", true],
    ['"con"', false],
    ["con or acme", false],
    ["-con", false],
    ["", false],
  ])("for %j answers %s", (typed, left) => {
    expect(mailLeftOut(typed)).toBe(left);
  });
});
