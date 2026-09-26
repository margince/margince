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
    expect(toQuery(composed, { scope_kind: "team", scope_id: "t-1" })).toEqual({
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
    const query = toQuery(newDraft("leads-by-status"), {});
    expect(query).not.toHaveProperty("group_by");
    expect(query).not.toHaveProperty("filters");
  });

  it("comes back from a saved query as the same draft", () => {
    const query = toQuery(composed, {});
    expect(toQuery(draftFromQuery(query), {})).toEqual(query);
  });
});

describe("why a draft cannot be asked yet", () => {
  it("wants a report first", () => {
    expect(draftProblem(newDraft(""))).toBe("analytics.q.needEntity");
  });

  it("wants a field for every aggregate but count", () => {
    expect(
      draftProblem({
        ...composed,
        measures: [{ id: 1, fn: "avg", field: "" }],
      }),
    ).toBe("analytics.q.needField");
  });

  it("wants a value for a comparison but not for a null test", () => {
    expect(
      draftProblem({
        ...composed,
        filters: [{ id: 3, field: "status", op: "eq", value: "" }],
      }),
    ).toBe("analytics.q.needValue");
    expect(draftProblem(composed)).toBeNull();
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
