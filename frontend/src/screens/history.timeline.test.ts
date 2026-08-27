import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { changeTimeline } from "./history";

// No record type reached by this list holds money today, so the context these
// rows are rendered in is "no currency" — which is itself the case worth
// pinning: a minor-unit value must never fall back to the bare integer.
const MONEY = { currency: null, locale: "en" as const, zone: "UTC" };

type FieldHistoryEntry = components["schemas"]["FieldHistoryEntry"];

// One audit row, three fields. The projection emits one entry per field and
// they all carry the AUDIT row's id, so the id alone does not identify a row.
const oneWriteThreeFields: FieldHistoryEntry[] = [
  "industry",
  "size_band",
  "legal_name",
].map((field) => ({
  id: "a-1",
  entity_type: "organization",
  entity_id: "o-1",
  field,
  old_value: null,
  new_value: "x",
  changed_at: "2026-07-14T10:00:00Z",
  actor_type: "human",
  // The spine stores the principal id, not the bare user id.
  actor_id: "human:u-1",
}));

describe("changeTimeline", () => {
  it("gives each field its own row identity within one audit write", () => {
    const rows = changeTimeline(
      oneWriteThreeFields,
      (field) => field,
      MONEY,
      "field updated",
    );
    expect(new Set(rows.map((row) => row.id)).size).toBe(3);
  });

  it("labels the field and keeps the change time", () => {
    const [row] = changeTimeline(
      oneWriteThreeFields,
      () => "Industry",
      MONEY,
      "field updated",
    );
    expect(row.title).toBe("Industry");
    expect(row.atIso).toBe("2026-07-14T10:00:00Z");
    expect(row.kind).toBe("change");
  });

  it("matches the reader through the principal prefix the spine stores", () => {
    const mine = changeTimeline(
      oneWriteThreeFields,
      (f) => f,
      MONEY,
      "field updated",
      "u-1",
    );
    expect(mine[0].provenance).toEqual({
      kind: "human",
      self: true,
      userId: "u-1",
    });
    const theirs = changeTimeline(
      oneWriteThreeFields,
      (f) => f,
      MONEY,
      "field updated",
      "u-2",
    );
    expect(theirs[0].provenance).toEqual({
      kind: "human",
      self: false,
      userId: "u-1",
    });
  });
});

// A minor-unit column is an integer count of the units its currency defines.
// Handing that integer to the diff shows a figure a hundred times the price on
// most currencies, and it is wrong silently — the number is plausible in both
// denominations, so nobody reading the row can tell. The per-field view already
// formatted these; this list did not, and the two showed one change two ways.
describe("a money field in the changes list", () => {
  const amountChange: FieldHistoryEntry[] = [
    {
      ...oneWriteThreeFields[0],
      field: "amount_minor",
      old_value: "150000",
      new_value: "225000",
    },
  ];

  it("renders the amount scaled by its currency, never the stored integer", () => {
    const [row] = changeTimeline(
      amountChange,
      (f) => f,
      { currency: "EUR", locale: "en", zone: "UTC" },
      "field updated",
    );
    const rendered = JSON.stringify(row.detail);
    expect(rendered).not.toContain("150000");
    expect(rendered).not.toContain("225000");
    expect(rendered).toContain("1,500");
    expect(rendered).toContain("2,250");
  });
});
