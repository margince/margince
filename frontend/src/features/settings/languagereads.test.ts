import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import { reloadReaderLanguageReads } from "./languagereads";

const READS: readonly (readonly unknown[])[] = [
  ["contact360", "c-1"],
  ["company360", "co-1"],
  ["reporting-catalog"],
  ["reporting-evaluation", { period: "this_month" }],
  ["reporting-live", "r-1", "published"],
];

describe("a display-language change", () => {
  it("marks every read the server words in the reader's language as stale", async () => {
    const client = new QueryClient();
    for (const key of [...READS, ["deals", "d-1"]]) {
      client.setQueryData(key, "cached");
    }
    await reloadReaderLanguageReads(client);
    for (const key of READS) {
      expect(client.getQueryState(key)?.isInvalidated, String(key[0])).toBe(
        true,
      );
    }
    expect(client.getQueryState(["deals", "d-1"])?.isInvalidated).toBe(false);
  });
});
