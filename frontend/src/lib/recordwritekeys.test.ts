// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import { ENTITY_KINDS } from "./entity";
import { invalidateCompany, recordWriteKeys } from "./recordwritekeys";

describe("recordWriteKeys", () => {
  it("answers for every record kind the history panel can be opened on", () => {
    for (const kind of ENTITY_KINDS) {
      expect(recordWriteKeys(kind, "r-1").length).toBeGreaterThan(0);
    }
  });

  it("names every sibling read of a contact, not one of the three", () => {
    // The three are siblings under no common prefix, so invalidating one
    // leaves the other two painting the state the reader just changed.
    expect(recordWriteKeys("contact", "p-1")).toEqual([
      ["contact", "p-1"],
      ["contact360", "p-1"],
      ["contactBrief", "p-1"],
      ["lists", "record", "contact", "p-1"],
    ]);
  });

  it("carries the lead's board and list, which its detail key does not reach", () => {
    expect(recordWriteKeys("lead", "l-1")).toContainEqual(["leads"]);
  });
});

describe("invalidateCompany", () => {
  it.each([
    { is_anchor: true, stale: true },
    { is_anchor: false, stale: false },
  ])(
    "stales the company profile and /me only when is_anchor is $is_anchor",
    async ({ is_anchor, stale }) => {
      const client = new QueryClient();
      const keys = [
        ["me"],
        ["company"],
        ["company", "c-1"],
        ["company", "c-2"],
      ];
      for (const queryKey of keys) client.setQueryData(queryKey, {});

      await invalidateCompany(client, { id: "c-1", is_anchor });

      const staled = (queryKey: string[]) =>
        client.getQueryState(queryKey)?.isInvalidated;
      expect(staled(["company", "c-1"])).toBe(true);
      expect(staled(["me"])).toBe(stale);
      expect(staled(["company"])).toBe(stale);
      expect(staled(["company", "c-2"])).toBe(false);
    },
  );
});
