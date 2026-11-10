// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir } from "../tree";
import { blueStatusLines, rule } from "./statusbadge.census";

describe("status palette fitness", () => {
  it.each([
    ["a JSX tone", `<Badge tone="info">x</Badge>`],
    ["a tone property", `const a = { tone: "accent", label: "x" };`],
    ["a tone union", `type T = "info" | "success" | undefined;`],
    ["a status map", `const M = { invited: "info", active: "success" };`],
  ])("catches %s", (_shape, code) => {
    expect(blueStatusLines(parseSource("planted.tsx", code))).toHaveLength(1);
  });

  it("leaves a blue Callout alone", () => {
    const code = `<Callout tone="info">x</Callout>`;
    expect(blueStatusLines(parseSource("planted.tsx", code))).toEqual([]);
  });

  it("leaves a map with no blue in it alone", () => {
    const code = `const M = { invited: "default", active: "success" };`;
    expect(blueStatusLines(parseSource("planted.tsx", code))).toEqual([]);
  });
});

describe("the statusbadge rule", () => {
  it("judges only a module that imports the palette", () => {
    const blue = `const M = { invited: "info", active: "success" };`;
    expect(
      plantedFindings(rule, [
        {
          path: join(srcDir, "features", "user.tsx"),
          area: "src",
          text: `import { StatusBadge } from "../design-system/statusbadge";\n${blue}`,
        },
        {
          path: join(srcDir, "features", "other.tsx"),
          area: "src",
          text: blue,
        },
      ]),
    ).toEqual(["src/features/user.tsx:2"]);
  });
});
