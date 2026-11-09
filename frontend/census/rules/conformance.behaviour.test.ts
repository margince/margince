// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import {
  defeatedReduceRules,
  emptyPlateOverrides,
  handSpelledGlows,
  handSpelledOutlines,
  onlyAt,
  plainRules,
  rule,
  unansweredAnimations,
} from "./conformance.behaviour.census";

const reduce = (body: string) =>
  `@media (prefers-reduced-motion: reduce) {\n  ${body}\n}\n`;

describe("reduced motion", () => {
  it("finds a reduce rule that a later rule of the same selector overrides", () => {
    const late = `${reduce(".x { transition: none; }")}.x { transition: width 1s; }\n`;
    expect(defeatedReduceRules("a.css", late)).toHaveLength(1);
    const early = `.x { transition: width 1s; }\n${reduce(".x { transition: none; }")}`;
    expect(defeatedReduceRules("a.css", early)).toEqual([]);
  });

  it("reads every rule after a media block, not every other one", () => {
    const text = `${reduce(".a { b: c; }")}.one { x: y; }\n.two { x: y; }\n`;
    expect(plainRules(text).map((rule) => rule.selector)).toEqual([
      ".one",
      ".two",
    ]);
  });

  it("asks each infinite animation for an answer on itself or an ancestor", () => {
    const pulse = ".feed .mote { animation: pulse 1s infinite; }\n";
    expect(unansweredAnimations("a.css", pulse)).toHaveLength(1);
    expect(
      unansweredAnimations("a.css", pulse + reduce(".feed { display: none; }")),
    ).toEqual([]);
  });
});

describe("focus", () => {
  it("finds a hand-spelled ring and glow, and passes the tokens", () => {
    expect(
      handSpelledOutlines("a.css", ".x { outline: 2px solid red; }"),
    ).toEqual(["a.css: outline: 2px solid red"]);
    expect(
      handSpelledOutlines(
        "a.css",
        "/* outline: 3px; */ .x { outline: var(--focus-ring); outline: none; }",
      ),
    ).toEqual([]);
    expect(
      handSpelledGlows("a.css", ".x:focus { box-shadow: 0 0 0 3px red; }"),
    ).toEqual(["a.css: box-shadow: 0 0 0 3px red"]);
    expect(
      handSpelledGlows("a.css", ".card { box-shadow: 0 1px 2px black; }"),
    ).toEqual([]);
  });
});

describe("the empty plate and the one-home lists", () => {
  it("finds a type class on the sentence inside EmptyState", () => {
    const text = 'x\n<EmptyState>\n  <p className="t-caption">{t("k")}</p>';
    expect(emptyPlateOverrides("a.tsx", text)).toEqual(["a.tsx:2"]);
    expect(emptyPlateOverrides("a.tsx", "<EmptyState>{t('k')}")).toEqual([]);
  });

  it("names an extra entry and a missing one", () => {
    expect(onlyAt(["a", "b"], ["a", "c"], "why")).toEqual([
      "b: why",
      "c is expected and was not found: why",
    ]);
  });
});

describe("the conformance.behaviour rule", () => {
  it("reports a planted second pending region through its own corpus", () => {
    const path = join(srcDir, "features", "planted.tsx");
    const text =
      'export const F = () => <div aria-busy={true} role="status" />;\n';
    const findings = plantedFindings(rule, [{ path, area: "src", text }]);
    expect(findings).toContain(
      "src/features/planted.tsx: a pending region comes from PendingBody in src/design-system/atoms.tsx",
    );
  });
});
