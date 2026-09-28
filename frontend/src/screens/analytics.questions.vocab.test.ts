// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { translate } from "../i18n";
import { ENTITIES, QUERY } from "./analytics.questions.testkit";
import {
  analyticsFieldLabel,
  columnLabel,
  explainGroup,
  fieldsForFn,
  fnsFor,
  measureReading,
  refusalOf,
  valueControlOp,
  valueShape,
} from "./analytics.questions.vocab";

const deals = ENTITIES[0];
const leads = ENTITIES[1];
const meetings = ENTITIES[2];
const t = (
  key: Parameters<typeof translate>[1],
  params?: Record<string, string>,
) => translate("en", key, params);

describe("which field an aggregate accepts", () => {
  it("gives count no field at all", () => {
    expect(fieldsForFn(deals, "count")).toEqual([]);
  });

  it("lets sum, avg, median and p75 read only a measure", () => {
    for (const fn of ["sum", "avg", "median", "p75"] as const) {
      expect(fieldsForFn(deals, fn)).toEqual(deals.measures);
    }
  });

  it("lets count_distinct, min and max read a dimension or a measure", () => {
    for (const fn of ["count_distinct", "min", "max"] as const) {
      expect(fieldsForFn(deals, fn)).toEqual([
        ...deals.group_by,
        ...deals.measures,
      ]);
    }
  });

  it("offers only count and the dimension readers on a population with no measures", () => {
    expect(fnsFor(leads)).toEqual(["count", "count_distinct", "min", "max"]);
  });
});

describe("what shape a filter value takes", () => {
  it("reads a measure and win_probability as numbers", () => {
    expect(valueShape(deals, "amount_minor")).toBe("number");
    expect(valueShape(deals, "win_probability")).toBe("number");
  });

  it("reads became_opportunity as yes or no, and everything else as text", () => {
    expect(valueShape(meetings, "became_opportunity")).toBe("boolean");
    expect(valueShape(deals, "stage_id")).toBe("text");
  });

  it("draws no value control for the two null tests", () => {
    expect(valueControlOp("is_null")).toBeNull();
    expect(valueControlOp("is_not_null")).toBeNull();
    expect(valueControlOp("ne")).toBe("neq");
    expect(valueControlOp("gte")).toBe("gte");
  });
});

describe("a column's name on screen", () => {
  it("names a group key by its field and a measure by its aggregate and field", () => {
    expect(columnLabel(t, QUERY, "stage_id")).toBe("Stage");
    expect(columnLabel(t, QUERY, "count")).toBe("Count");
    expect(columnLabel(t, QUERY, "sum_amount_minor")).toBe("Sum (Amount)");
  });

  it("falls back to the wire name for a field the catalog does not carry", () => {
    expect(analyticsFieldLabel(t, "brand_new_field")).toBe("brand_new_field");
  });
});

describe("how a money cell reads", () => {
  const sum = { fn: "sum" as const, field: "amount_minor" };

  it("uses the row's own currency when the question groups by it", () => {
    expect(measureReading(sum, { currency: "USD" }, QUERY, "EUR")).toEqual({
      kind: "money",
      currency: "USD",
    });
  });

  it("uses the currency a filter pinned when there is no currency column", () => {
    const pinned = {
      ...QUERY,
      group_by: ["stage_id"],
      filters: [{ field: "currency", op: "eq" as const, value: "VND" }],
    };
    expect(measureReading(sum, {}, pinned, "EUR")).toEqual({
      kind: "money",
      currency: "VND",
    });
  });

  it("refuses a figure for native amounts summed across currencies", () => {
    expect(
      measureReading(sum, {}, { ...QUERY, group_by: ["stage_id"] }, "EUR"),
    ).toEqual({ kind: "mixed" });
  });

  it("reads a converted amount in the base currency", () => {
    expect(
      measureReading(
        { fn: "sum", field: "amount_base_minor" },
        {},
        QUERY,
        "EUR",
      ),
    ).toEqual({ kind: "money", currency: "EUR" });
  });

  it("counts distinct amounts as a number, not money", () => {
    expect(
      measureReading(
        { fn: "count_distinct", field: "amount_minor" },
        {},
        QUERY,
        "EUR",
      ),
    ).toEqual({ kind: "number" });
  });
});

describe("the cell a row names for its drill-down", () => {
  it("lists the group keys in the question's own order, null for an unset one", () => {
    expect(
      explainGroup(QUERY, { currency: "EUR", stage_id: null, count: 3 }),
    ).toEqual([null, "EUR"]);
  });

  it("names no group for an ungrouped answer", () => {
    expect(explainGroup({ ...QUERY, group_by: [] }, { count: 3 })).toBe(
      undefined,
    );
  });
});

describe("a refusal read off the problem body", () => {
  it("reads the kind, suggestion and message from the structured details", () => {
    expect(
      refusalOf({
        code: "invalid_argument",
        detail: "privacy: too few records — group by stage",
        details: {
          kind: "privacy",
          message: "too few records",
          suggest: "group by stage",
        },
      }),
    ).toEqual({
      kind: "privacy",
      suggest: "group by stage",
      message: "too few records",
    });
  });

  it("is not a refusal without structured details, or of a kind with no words", () => {
    expect(refusalOf({ code: "permission_denied", detail: "no" })).toBeNull();
    expect(
      refusalOf({
        details: { kind: "too_expensive", message: "m", suggest: "s" },
      }),
    ).toBeNull();
  });
});

describe("a converted amount with no base currency", () => {
  it("reads as absent, never as a mix of currencies", () => {
    expect(
      measureReading(
        { fn: "sum", field: "amount_base_minor" },
        {},
        QUERY,
        null,
      ),
    ).toEqual({ kind: "noBase" });
  });
});
