// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { asked, declared, rule } from "./typeclass-coverage.census";

describe("the type scale", () => {
  it("reads every spelling of a className", () => {
    const text = [
      '<b className="t-label x" />',
      '<b className={"t-num"} />',
      "<b className={'t-caption'} />",
      `<b className={\`t-h2 \${tone}\`} />`,
      `<b className={\`t-\${size}\`} />`,
    ].join("\n");
    expect(asked("a.tsx", text)).toEqual([
      ["t-label", "a.tsx:1"],
      ["t-num", "a.tsx:2"],
      ["t-caption", "a.tsx:3"],
      ["t-h2", "a.tsx:4"],
    ]);
  });

  it("reads the classes a stylesheet declares", () => {
    expect(declared(".t-label, .x .t-num { }")).toEqual(["t-label", "t-num"]);
  });

  it("reports a planted undeclared class through its own corpus and across", () => {
    expect(
      plantedFindings(rule, [
        {
          path: join(srcDir, "features", "planted.tsx"),
          area: "src",
          text: 'const a = <b className="t-meta t-label" />;',
        },
        {
          path: join(srcDir, "design-system", "base.css"),
          area: "src",
          text: ".t-label { }",
        },
      ]),
    ).toEqual(["t-meta: src/features/planted.tsx:1"]);
  });
});
