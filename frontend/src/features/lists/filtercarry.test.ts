// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { carriedValue } from "./filtercarry";

describe("the value a clause keeps when its operator changes", () => {
  it("keeps a typed string or number between comparisons", () => {
    expect(carriedValue("contains", "eq", "Contact 0", "text")).toBe(
      "Contact 0",
    );
    expect(carriedValue("eq", "contains", "Contact 0", "text")).toBe(
      "Contact 0",
    );
    expect(carriedValue("gte", "lt", 12.5, "number")).toBe(12.5);
    expect(carriedValue("eq", "gt", 125000, "currency")).toBe(125000);
  });

  it("keeps a count of days between date comparisons", () => {
    const week = { days_ago: 7 };
    expect(carriedValue("eq", "gte", week, "date")).toBe(week);
    expect(carriedValue("lt", "neq", "2026-01-31", "date")).toBe("2026-01-31");
  });

  it("starts a list with the value already chosen", () => {
    expect(carriedValue("eq", "in", "gold", "picklist")).toEqual(["gold"]);
    expect(carriedValue("eq", "in", "u-1", "id")).toEqual(["u-1"]);
    expect(carriedValue("eq", "in", 12, "number")).toEqual([12]);
    expect(carriedValue("eq", "in", "", "text")).toEqual([]);
  });

  it("keeps a list's item only when the list holds exactly one", () => {
    expect(carriedValue("in", "eq", ["u-1"], "id")).toBe("u-1");
    expect(carriedValue("in", "neq", [12], "number")).toBe(12);
    expect(carriedValue("in", "eq", ["u-1", "u-2"], "id")).toBe("");
    expect(carriedValue("in", "eq", [], "text")).toBe("");
  });

  it("carries a number, and no typed text, into or out of a number list", () => {
    expect(carriedValue("eq", "in", 12, "number")).toEqual([12]);
    expect(carriedValue("in", "gte", [12], "number")).toBe(12);
    expect(carriedValue("eq", "in", "1.500", "number")).toEqual([]);
    expect(carriedValue("in", "eq", ["12"], "number")).toBe("");
    expect(carriedValue("eq", "in", "abc", "currency")).toEqual([]);
  });

  it("gives `contains` only a string", () => {
    expect(carriedValue("eq", "contains", 12, "text")).toBe("");
  });

  it("resets a value no control of the new operator can show", () => {
    expect(carriedValue("eq", "in", { days_ago: 7 }, "date")).toEqual([]);
    expect(carriedValue("eq", "exists", "ann", "text")).toBe(true);
    expect(carriedValue("exists", "eq", false, "text")).toBe("");
    expect(carriedValue("within_radius", "eq", "x", "distance")).toBe("");
    expect(carriedValue("eq", "within_radius", "x", "distance")).toBe("");
  });

  it("starts a yes/no field on the arm its control shows", () => {
    expect(carriedValue("eq", "neq", false, "boolean")).toBe(true);
  });
});
