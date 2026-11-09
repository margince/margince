// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir } from "../tree";
import { castsIn, castsToUnknown, rule } from "./contractcasts.census";

// The detector, asked about every shape it has to read.
//
// The census reports only what this recognises. A reader that stopped seeing
// one shape would pass the whole gate. Two cases below are ways it was wrong
// before: a type imported rather than declared, and a waiver written as a
// paragraph.
describe("what counts as a contract type cast", () => {
  const found = (code: string) =>
    castsIn(parseSource(probePath, code), "src/features/worklist/probe.ts").map(
      (one) => one.says,
    );

  // A real module, so the import arm resolves against a file that exists. This
  // one declares `Worklist` as a contract alias and re-exports it, which is the
  // shape the screens use.
  const probePath = join(srcDir, "features", "worklist", "probe.ts");
  const aliasing = 'import type { Worklist } from "./worklist.queries";\n';

  it("finds a cast to a type the module declares", () => {
    expect(
      found(
        'type Deal = components["schemas"]["Deal"];\nconst d = {} as unknown as Deal;',
      ),
    ).toEqual(["`as unknown as Deal`"]);
  });

  it("finds a cast to a contract type the module imported", () => {
    expect(found(`${aliasing}const w = {} as unknown as Worklist;`)).toEqual([
      "`as unknown as Worklist`",
    ]);
  });

  // SPACING IS NOT MEANING. `components ["schemas"]["Deal"]` is the same type,
  // and the reader that demanded the bracket sit against the identifier called
  // it something other than the contract.
  it("finds a cast to the contract however it is spaced", () => {
    expect(
      found('const o = {} as unknown as components ["schemas"]["Offer"];'),
    ).toEqual(['`as unknown as components ["schemas"]["Offer"]`']);
  });

  // Brackets round the bridge are still the bridge. A reader that wanted the
  // inner `as unknown` as the direct child of the outer cast passed over this.
  it("finds a cast whose unknown bridge is parenthesised", () => {
    expect(found(`${aliasing}const w = ({} as unknown) as Worklist;`)).toEqual([
      "`as unknown as Worklist`",
    ]);
  });

  it("finds a cast through a modifier on a contract type", () => {
    expect(
      found(`${aliasing}const w = {} as unknown as Partial<Worklist>;`),
    ).toEqual(["`as unknown as Partial<Worklist>`"]);
    expect(
      found('const o = {} as unknown as components["schemas"]["Offer"];'),
    ).toEqual(['`as unknown as components["schemas"]["Offer"]`']);
  });

  // The DOM, the router and the shape of a mock are a different question. A cast
  // to one of those says nothing about whether a fixture matches the wire.
  it("passes over a cast to something that is not the contract", () => {
    expect(found("const e = {} as unknown as Event;")).toEqual([]);
    expect(found("const m = fetch as unknown as { calls: number };")).toEqual(
      [],
    );
  });

  // A waiver is read on the lines TypeScript counts, whatever ends them.
  it("finds a reasoned waiver above a cast at every line terminator", () => {
    for (const end of ["\r\n", "\r", "\u2028", "\u2029"]) {
      const code = `${aliasing}// contract:cast \`{}\` is not a page, on purpose.${end}const w = {} as unknown as Worklist;`;
      expect(found(code), JSON.stringify(end)).toEqual([]);
    }
  });

  it("takes a reason from anywhere in the comment block above", () => {
    expect(
      found(
        `${aliasing}// contract:cast \`{}\` is not a page, on purpose.\n// The case is about what the reader survives.\nconst w = {} as unknown as Worklist;`,
      ),
    ).toEqual([]);
  });

  // The census parses only the modules this admits, so every shape of the
  // bridge the parser reads must pass it.
  it("parses every module that spells the bridge, trivia and all", () => {
    for (const bridge of [
      "{} as unknown as Worklist",
      "({} as\n  unknown) as Worklist",
      "{} as /* the wire */ unknown as Worklist",
      "{} as // the wire\n  unknown as Worklist",
      "{} as // the wire\r  unknown as Worklist",
      "{} as // the wire\u2028  unknown as Worklist",
      "{} as // the wire\u2029  unknown as Worklist",
    ]) {
      const code = `${aliasing}const w = ${bridge};`;
      expect(castsToUnknown(code), bridge).toBe(true);
      expect(found(code), bridge).toHaveLength(1);
    }
    expect(castsToUnknown("const u: unknown = value as Worklist;")).toBe(false);
  });

  // A marker with no reason says a check was skipped and not why.
  it("refuses a marker with no reason", () => {
    expect(
      found(
        `${aliasing}// contract:cast\nconst w = {} as unknown as Worklist;`,
      ),
    ).toEqual([
      "`contract:cast` with no reason beside `as unknown as Worklist`",
    ]);
  });
});

describe("the contractcasts rule", () => {
  it("reports a planted cast through its own corpus and visit", () => {
    const path = join(srcDir, "features", "planted.ts");
    const text =
      'type Deal = components["schemas"]["Deal"];\nconst d = {} as unknown as Deal;\n';
    expect(plantedFindings(rule, [{ path, area: "src", text }])).toEqual([
      "src/features/planted.ts:2 `as unknown as Deal`",
    ]);
  });
});
