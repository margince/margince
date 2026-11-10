// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir } from "../tree";
import { rule, unwrappedIn } from "./table-scroll-coverage.census";

describe("a hand-drawn table", () => {
  const planted: ReadonlyArray<readonly [string, string, number]> = [
    ["a bare table", "const a = <table><tbody /></table>;", 1],
    [
      "a table under a hand-written scroll div, which is the defect",
      'const a = <div className="import__scroll"><table><tbody /></table></div>;',
      1,
    ],
    [
      "two bare tables, which a per-file flag would count once",
      "const a = <><table><tbody /></table><table><tbody /></table></>;",
      2,
    ],
    [
      "a table inside TableScroll",
      'const a = <TableScroll label="x"><table><tbody /></table></TableScroll>;',
      0,
    ],
    [
      "a table several levels down inside TableScroll",
      'const a = <TableScroll label="x"><div><section><table><tbody /></table></section></div></TableScroll>;',
      0,
    ],
    [
      "a TableScroll that wraps something else entirely",
      'const a = <><TableScroll label="x"><p /></TableScroll><DataTable rows={r} /></>;',
      0,
    ],
    [
      "a table drawn only for a screen reader",
      'const a = <table className="sr-only"><tbody /></table>;',
      0,
    ],
  ];
  for (const [name, source, expected] of planted) {
    it(`sees ${name}`, () => {
      expect(unwrappedIn(parseSource("planted.tsx", source))).toHaveLength(
        expected,
      );
    });
  }

  it("reports a planted bare table through its own corpus and visit", () => {
    const text = "const a = <table><tbody /></table>;";
    expect(
      plantedFindings(rule, [
        { path: join(srcDir, "features", "planted.tsx"), area: "src", text },
        {
          path: join(srcDir, "design-system", "listgrid.tsx"),
          area: "src",
          text,
        },
      ]),
    ).toEqual(["src/features/planted.tsx:1"]);
  });
});
