import { describe, expect, it } from "vitest";
import { entryParams, entryRecordFromParams } from "./extensionentries";

describe("the record an entry hands a unit screen", () => {
  it("reads back what the entry wrote", () => {
    const record = { type: "deal", id: "d-7" } as const;
    expect(entryRecordFromParams(entryParams(record))).toEqual(record);
  });

  it("reads a kind this app does not route to as no record", () => {
    const params = new Map([
      ["record_type", "activity"],
      ["record_id", "a-1"],
    ]);
    expect(entryRecordFromParams(params)).toBeNull();
  });

  it("reads an address with no record as no record", () => {
    expect(entryRecordFromParams(entryParams(undefined))).toBeNull();
  });
});
