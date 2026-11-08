// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { en } from "../../src/i18n/en";
import { srcDir as srcRoot } from "../tree";
import { findingsIn } from "./one-plural-rule.census";

describe("one plural rule", () => {
  // The gate's own census. A gate that matches a shape the defect does not take
  // reports PASS, so each spelling is planted here.
  it("sees the defect in every spelling it takes", () => {
    const both = Object.keys(en).slice(0, 2);
    const [first, second] = both;
    const cases = [
      `const label = n === 1 ? "${first}" : "${second}";`,
      `const label = 1 === n ? "${first}" : "${second}";`,
      `const label = n !== 1 ? "${first}" : "${second}";`,
      `const label = n > 1 ? "${first}" : "${second}";`,
      `const label = t(rows === 1 ? "${first}" : "${second}", {});`,
      `const label = n === 1 ? \`${first}\` : \`${second}\`;`,
      // The arms carrying the keys rather than the condition being inside the
      // call. This is the spelling the tree actually used, and the one the gate
      // read straight past on its first arming.
      `const label = n === 1 ? t("${first}") : t("${second}", { count });`,
      // A translator that takes the locale first, so the key is not argument
      // one. Matching a fixed position would be hard-coding the subject.
      `const label = n === 1 ? translate(locale, "${first}") : translate(locale, "${second}");`,
      `const label = n === 1 ? (t("${first}")) : (t("${second}"));`,
    ];
    for (const source of cases) {
      expect(
        findingsIn(join(srcRoot, "planted.ts"), source),
        `not seen: ${source}`,
      ).toHaveLength(1);
    }
  });

  it("rules on nothing it has no standing to rule on", () => {
    const both = Object.keys(en).slice(0, 2);
    const [first, second] = both;
    const allowed = [
      // A state choice, not a plural one.
      `const label = isPaid ? "${first}" : "${second}";`,
      // A count comparison that produces no keys.
      `const width = n === 1 ? 10 : 20;`,
      // Key-shaped strings the catalogue does not carry are not this gate's
      // business.
      `const label = n === 1 ? "not.a.key" : "also.not.a.key";`,
      // Half a pair. One arm being a key is not a choice between wordings.
      `const label = n === 1 ? "${first}" : someOtherThing;`,
      // A call on both arms with no key: a count picking between two computations
      // is not a wording choice.
      `const label = n === 1 ? formatOne(value) : formatMany(value);`,
      // A waived site: the author says why, on the line above.
      `// plural-rule:allow one open deal needs no name\nconst label = n === 1 ? t("${first}") : t("${second}");`,
    ];
    for (const source of allowed) {
      expect(
        findingsIn(join(srcRoot, "planted.ts"), source),
        `wrongly flagged: ${source}`,
      ).toHaveLength(0);
    }
  });

  it("does not accept a waiver with no reason", () => {
    // The waiver has to cost a sentence. A bare marker is a keystroke that
    // turns the gate off, and a gate with one of those is decoration.
    const [first, second] = Object.keys(en).slice(0, 2);
    const reasonless = [
      `// plural-rule:allow\nconst label = n === 1 ? t("${first}") : t("${second}");`,
      `// plural-rule:allow   \nconst label = n === 1 ? t("${first}") : t("${second}");`,
    ];
    for (const source of reasonless) {
      expect(
        findingsIn(join(srcRoot, "planted.ts"), source),
        `a reasonless waiver was honoured: ${source}`,
      ).toHaveLength(1);
    }
  });

  it("waives the site below the comment and not the next one down", () => {
    // The walk up stops at the first line of code, so the second site here is
    // still a finding.
    const [first, second] = Object.keys(en).slice(0, 2);
    const source = [
      `// plural-rule:allow this one is a naming choice`,
      `const first = n === 1 ? t("${first}") : t("${second}");`,
      `const second = n === 1 ? t("${first}") : t("${second}");`,
    ].join("\n");
    expect(findingsIn(join(srcRoot, "planted.ts"), source)).toHaveLength(1);
  });

  it("still reads a waiver whose reason wraps, and a split conditional", () => {
    // The formatter owns where lines break, so a waiver must hold when its reason
    // wraps and the conditional splits over lines.
    const [first, second] = Object.keys(en).slice(0, 2);
    const source = [
      `// plural-rule:allow naming the thing is a choice between two`,
      `// sentences rather than between two forms of one`,
      `const label =`,
      `  n === 1`,
      `    ? t("${first}")`,
      `    : t("${second}");`,
    ].join("\n");
    expect(findingsIn(join(srcRoot, "planted.ts"), source)).toHaveLength(0);
  });
});
