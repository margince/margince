import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { countFindings, judge, LEDGER, parseLedger } from "./compiler-lint.ts";

const result = (filePath: string, ...ruleIds: (string | null)[]) => ({
  filePath,
  messages: ruleIds.map((ruleId) => ({ ruleId })),
});

describe("the React Compiler lint ratchet", () => {
  it("counts findings per file and rule, relative to the frontend", () => {
    const counts = countFindings(
      [
        result("/fe/src/a.tsx", "react-hooks/refs", "react-hooks/refs"),
        result("/fe/src/b.ts", null),
      ],
      "/fe",
    );
    expect([...counts]).toEqual([
      ["src/a.tsx react-hooks/refs", 2],
      ["src/b.ts parse-error", 1],
    ]);
  });

  it("passes when every count matches the ledger", () => {
    const counts = new Map([["src/a.tsx react-hooks/refs", 2]]);
    expect(judge(counts, parseLedger("2 react-hooks/refs src/a.tsx"))).toEqual(
      [],
    );
  });

  it("refuses a new finding in a file the ledger does not name", () => {
    const counts = new Map([["src/new.tsx react-hooks/purity", 1]]);
    expect(judge(counts, new Map())).toEqual([
      expect.stringContaining(
        "src/new.tsx react-hooks/purity: 1 finding(s), ledger allows 0",
      ),
    ]);
  });

  it("refuses a count that grew past its frozen value", () => {
    const counts = new Map([["src/a.tsx react-hooks/refs", 3]]);
    expect(judge(counts, parseLedger("2 react-hooks/refs src/a.tsx"))).toEqual([
      expect.stringContaining("3 finding(s), ledger allows 2"),
    ]);
  });

  it("asks for the ledger to follow a count down", () => {
    const counts = new Map([["src/a.tsx react-hooks/refs", 1]]);
    expect(judge(counts, parseLedger("2 react-hooks/refs src/a.tsx"))).toEqual([
      expect.stringContaining("down to 1 from 2"),
    ]);
  });

  it("asks for a fixed entry to be deleted", () => {
    expect(
      judge(new Map(), parseLedger("2 react-hooks/refs src/a.tsx")),
    ).toEqual([expect.stringContaining("fixed (ledger had 2)")]);
  });

  it("refuses a malformed ledger line", () => {
    expect(() => parseLedger("0 react-hooks/refs src/a.tsx")).toThrow(
      /expected "<count> <rule> <path>"/,
    );
    expect(() => parseLedger("2 react-hooks/refs")).toThrow();
  });

  it("reads the committed ledger", () => {
    expect(parseLedger(readFileSync(LEDGER, "utf8")).size).toBeGreaterThan(0);
  });
});
