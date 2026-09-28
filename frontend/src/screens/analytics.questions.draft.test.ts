// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  draftFromQuery,
  draftProblem,
  newDraft,
  type QuestionDraft,
  retarget,
  toQuery,
} from "./analytics.questions.draft";
import { ENTITIES } from "./analytics.questions.testkit";

const [deals, leads] = ENTITIES;

const composed: QuestionDraft = {
  entity: "deals-by-stage",
  groupBy: ["stage_id", "currency"],
  measures: [
    { id: 1, fn: "count", field: "" },
    { id: 2, fn: "sum", field: "amount_minor" },
  ],
  filters: [
    { id: 3, field: "win_probability", op: "gte", value: 50 },
    { id: 4, field: "partner_company_id", op: "is_null", value: "" },
  ],
};

describe("a draft becoming the query the engine reads", () => {
  it("sends count with no field, the scope, and the limit it will report", () => {
    expect(
      toQuery(composed, { scope_kind: "team", scope_id: "t-1" }, "EUR"),
    ).toEqual({
      entity: "deals-by-stage",
      scope_kind: "team",
      scope_id: "t-1",
      group_by: ["stage_id", "currency"],
      measures: [{ fn: "count" }, { fn: "sum", field: "amount_minor" }],
      filters: [
        { field: "win_probability", op: "gte", value: 50 },
        { field: "partner_company_id", op: "is_null" },
      ],
      limit: 100,
    });
  });

  it("leaves out an empty grouping and an empty filter list", () => {
    const query = toQuery(newDraft("leads-by-status"), {}, "EUR");
    expect(query).not.toHaveProperty("group_by");
    expect(query).not.toHaveProperty("filters");
  });

  it("comes back from a saved query as the same draft", () => {
    const query = toQuery(composed, {}, "EUR");
    expect(toQuery(draftFromQuery(query, "EUR"), {}, "EUR")).toEqual(query);
  });
});

describe("why a draft cannot be asked yet", () => {
  it("wants a report first", () => {
    expect(draftProblem(newDraft(""), "EUR")).toBe("analytics.q.needEntity");
  });

  it("wants a field for every aggregate but count", () => {
    expect(
      draftProblem(
        { ...composed, measures: [{ id: 1, fn: "avg", field: "" }] },
        "EUR",
      ),
    ).toBe("analytics.q.needField");
  });

  it("wants a value for a comparison but not for a null test", () => {
    expect(
      draftProblem(
        {
          ...composed,
          filters: [{ id: 3, field: "status", op: "eq", value: "" }],
        },
        "EUR",
      ),
    ).toBe("analytics.q.needValue");
    expect(draftProblem(composed, "EUR")).toBeNull();
  });
});

describe("moving a draft onto another report", () => {
  it("drops the fields the new report does not have and keeps the rest", () => {
    const moved = retarget(
      { ...composed, groupBy: ["stage_id", "status"] },
      leads,
    );
    expect(moved.entity).toBe("leads-by-status");
    expect(moved.groupBy).toEqual(["status"]);
    expect(moved.measures).toEqual([{ id: 1, fn: "count", field: "" }]);
    expect(moved.filters).toEqual([]);
  });

  it("falls back to a count when no measure survives", () => {
    const moved = retarget(
      { ...composed, measures: [{ id: 2, fn: "sum", field: "amount_minor" }] },
      leads,
    );
    expect(moved.measures).toEqual([{ id: 1, fn: "count", field: "" }]);
  });

  it("keeps everything on the report it came from, grouped in its field order", () => {
    expect(retarget(composed, deals)).toEqual({
      ...composed,
      groupBy: ["currency", "stage_id"],
    });
  });
});

describe("an amount in a filter", () => {
  const withFilters = (filters: QuestionDraft["filters"]): QuestionDraft => ({
    ...composed,
    filters,
  });

  it("is typed in major units and sent in the base currency's minor ones", () => {
    const draft = withFilters([
      { id: 3, field: "amount_base_minor", op: "gte", value: 5000 },
    ]);
    expect(draftProblem(draft, "EUR")).toBeNull();
    expect(toQuery(draft, {}, "EUR").filters).toEqual([
      { field: "amount_base_minor", op: "gte", value: 500_000 },
    ]);
  });

  it("scales a deal's own amount by the one currency a filter pins", () => {
    const draft = withFilters([
      { id: 3, field: "currency", op: "eq", value: "VND" },
      { id: 4, field: "amount_minor", op: "gte", value: 5000 },
    ]);
    expect(draftProblem(draft, "EUR")).toBeNull();
    expect(toQuery(draft, {}, "EUR").filters?.[1]).toEqual({
      field: "amount_minor",
      op: "gte",
      value: 5000,
    });
  });

  it("refuses a deal's own amount until one currency is pinned", () => {
    const draft = withFilters([
      { id: 3, field: "amount_minor", op: "gte", value: 5000 },
    ]);
    expect(draftProblem(draft, "EUR")).toBe("analytics.q.needCurrencyFilter");
  });

  it("refuses a converted amount with no base currency, and a figure the currency cannot hold", () => {
    const converted = withFilters([
      { id: 3, field: "amount_base_minor", op: "gte", value: 5000 },
    ]);
    expect(draftProblem(converted, null)).toBe("analytics.noBaseCurrencyWhy");
    const tooFine = withFilters([
      { id: 3, field: "amount_base_minor", op: "gte", value: 50.001 },
    ]);
    expect(draftProblem(tooFine, "EUR")).toBe("analytics.q.amountInvalid");
  });

  it("comes back from a saved question in the major units the reader typed", () => {
    const query = toQuery(
      withFilters([
        { id: 3, field: "amount_base_minor", op: "gte", value: 5000 },
      ]),
      {},
      "EUR",
    );
    expect(draftFromQuery(query, "EUR").filters[0]?.value).toBe(5000);
  });
});
