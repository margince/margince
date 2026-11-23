import { describe, expect, it } from "vitest";
import {
  entryParams,
  entryRecordFromParams,
  extensionRecordHref,
} from "./extensionentries";
import { parseParams } from "./urlstate";

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

describe("the address of a unit's own record", () => {
  it("opens the unit's screen at the declared view with the record in the query", () => {
    expect(extensionRecordHref("ext:rooms:room", "r-1", "room-page")).toBe(
      "#/ext/rooms/room-page?record_id=r-1&record_type=ext%3Arooms%3Aroom",
    );
  });

  it("opens the unit's screen itself when the kind declared no view", () => {
    expect(extensionRecordHref("ext:rooms:room", "r-1", undefined)).toMatch(
      /^#\/ext\/rooms\?/,
    );
  });

  it("reads back as the same record on the unit's screen", () => {
    const query = extensionRecordHref("ext:rooms:room", "r-1", "room-page");
    expect(entryRecordFromParams(parseParams(query))).toEqual({
      type: "ext:rooms:room",
      id: "r-1",
    });
  });
});
