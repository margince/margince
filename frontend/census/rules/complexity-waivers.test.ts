// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { rule, unnamedWaivers } from "./complexity-waivers.census";

const unnamed =
  "// biome-ignore lint/complexity/noExcessiveCognitiveComplexity lint/complexity/noExcessiveLinesPerFunction: one surface";

describe("the complexity-waivers census", () => {
  it("tells a named waiver from an unnamed one", () => {
    const named =
      "// biome-ignore lint/complexity/noExcessiveLinesPerFunction: splitting it is #7201";
    const other =
      "// biome-ignore lint/suspicious/noArrayIndexKey: the position is the identity";
    expect(unnamedWaivers(`${named}\n${unnamed}\n${other}`)).toEqual([2]);
  });
});

describe("the complexity-waivers rule", () => {
  it("reports a planted unnamed waiver in production code", () => {
    const path = join(srcDir, "features", "planted.tsx");
    expect(
      plantedFindings(rule, [{ path, area: "src", text: `\n${unnamed}\n` }]),
    ).toEqual(["src/features/planted.tsx:2"]);
  });

  it("leaves test code alone", () => {
    const planted = ["features/planted.test.tsx", "testing/planted.tsx"].map(
      (name) => ({
        path: join(srcDir, name),
        area: "src" as const,
        text: unnamed,
      }),
    );
    expect(plantedFindings(rule, planted)).toEqual([]);
  });
});
