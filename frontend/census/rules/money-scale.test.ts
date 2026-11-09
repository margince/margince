// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir } from "../tree";
import { corpusPath, rule, scaleFindings } from "./money-scale.census";

type PlantedCase = {
  gate: string;
  lang: string;
  arm: string;
  want: "fires" | "silent" | "unclosed";
  name: string;
  body: string;
};

const planted: PlantedCase[] = JSON.parse(
  readFileSync(corpusPath, "utf8"),
).filter((one: PlantedCase) => one.gate === "money-scale" && one.lang === "ts");

describe("the money-scale census, over the cases planted for it", () => {
  it("has cases to run", () => {
    expect(planted.length).toBeGreaterThan(0);
  });

  for (const one of planted) {
    it(`${one.want}: ${one.name}`, () => {
      const { findings, unreadable } = scaleFindings(
        parseSource(`probe-${one.arm}-${one.name}.ts`, `${one.body}\n`),
      );
      // `unclosed` says the census could not read the code and refused, which
      // is the opposite of `fires`.
      if (one.want === "unclosed") {
        expect(unreadable).toBe(true);
        return;
      }
      expect(unreadable).toBe(false);
      if (one.want === "fires") {
        expect(
          findings.length,
          `the planted defect was not found`,
        ).toBeGreaterThan(0);
      } else {
        expect(
          findings,
          `the census refused code that is not the defect`,
        ).toEqual([]);
      }
    });
  }
});

describe("the money-scale rule", () => {
  const scaled =
    "export const major = (amountMinor: number) => amountMinor / 100;\n";

  it("reports a screen and passes the owner and a test", () => {
    expect(
      plantedFindings(rule, [
        { path: join(srcDir, "features", "x.ts"), area: "src", text: scaled },
        {
          path: join(srcDir, "format", "minorunits.ts"),
          area: "src",
          text: scaled,
        },
        {
          path: join(srcDir, "features", "x.test.ts"),
          area: "src",
          text: scaled,
        },
      ]),
    ).toEqual(["src/features/x.ts:1: amountMinor / 100"]);
  });

  it("reports a file it could not finish reading", () => {
    expect(
      plantedFindings(rule, [
        {
          path: join(srcDir, "features", "y.ts"),
          area: "src",
          text: "const a = `open\n",
        },
      ]),
    ).toEqual(["src/features/y.ts: the parser could not finish it"]);
  });
});
