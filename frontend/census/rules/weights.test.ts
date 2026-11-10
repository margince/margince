// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { frontendRoot, plantedFindings, srcDir } from "../tree";
import {
  declaredFamily,
  requested,
  rule,
  SHIPPED,
  TOKENS,
  usesIn,
  weightTokens,
} from "./weights.census";

describe("three weights, and every one of them drawn", () => {
  const request = requested();
  const tokens = weightTokens(readFileSync(join(frontendRoot, TOKENS), "utf8"));

  // A role token on a face nobody loaded is synthesized like a spelled 600,
  // at every call site at once.
  it("declares no weight token the request does not load", () => {
    const rogue = [...tokens]
      .filter(([, weight]) => !SHIPPED.includes(weight))
      .map(([name, weight]) => `${name} is ${weight}`);
    expect(rogue, rogue.join("\n")).toEqual([]);
  });

  it("loads exactly 400, 500 and 700 for both text families", () => {
    for (const token of ["fontFamilyHeading", "fontFamilyBody"]) {
      const family = declaredFamily(token);
      expect(request.get(family), family).toEqual([...SHIPPED]);
    }
  });

  it("loads the code face in none but those weights", () => {
    const mono = declaredFamily("fontFamilyMono");
    const loaded = request.get(mono);
    expect(loaded, `${mono} is not requested`).toBeDefined();
    expect(loaded?.filter((weight) => !SHIPPED.includes(weight))).toEqual([]);
  });

  it("reads a weight however a stylesheet or a style object asks", () => {
    const asked = (file: string, text: string) =>
      usesIn(file, text, tokens).map(({ weight }) => weight);
    expect(asked("a.css", ".a { font-weight: 600; }")).toEqual([600]);
    expect(asked("a.css", ".a { font: bold 1rem/1 X; }")).toEqual([700]);
    expect(asked("a.css", "/* font-weight: 300 */ .a { }")).toEqual([]);
    expect(
      asked("a.tsx", 'const s = { fontWeight: "var(--fontWeightBold)" };'),
    ).toEqual([tokens.get("--fontWeightBold")]);
  });
});

describe("the weights rule", () => {
  it("reports a planted synthesized weight through its own corpus and across", () => {
    expect(
      plantedFindings(rule, [
        {
          path: join(srcDir, "features", "planted.css"),
          area: "src",
          text: ".a {\n  font-weight: 600;\n}",
        },
      ]),
    ).toEqual(["src/features/planted.css:2 asks for 600"]);
  });
});
