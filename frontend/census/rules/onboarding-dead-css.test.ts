// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { srcDir, treeOf } from "../tree";
import {
  code,
  deadCss,
  declaredClasses,
  hollowRules,
  rule,
} from "./onboarding-dead-css.census";

const dir = join(srcDir, "features", "onboarding");

// The two composing sites the runtime list names, as the tree spells them.
const composers = [
  {
    path: join(dir, "onboarding-gate.tsx"),
    // biome-ignore lint/suspicious/noTemplateCurlyInString: a fixture of source code
    code: 'cls(`is-${notice.tone}`); "error"; "paused"; "resumed";',
  },
  {
    path: join(srcDir, "components", "trust.tsx"),
    // biome-ignore lint/suspicious/noTemplateCurlyInString: a fixture of source code
    code: 'cls(`confidence-${level}`); "high"; "med"; "low";',
  },
];

describe("the onboarding-dead-css rule", () => {
  it("reads classes, prose and empty bodies the way the rule needs", () => {
    expect(declaredClasses(".a-b:hover .c { x: 1 } /* .gone */")).toEqual([
      "a-b",
      "c",
    ]);
    expect(code("x /* .dropzone */\n  // .dz-x\nhttp://a.b")).not.toMatch(
      /dropzone|dz-x/,
    );
    expect(hollowRules(".a { } .b { color: red; } .c {/* x */}")).toEqual([
      ".a",
      ".c",
    ]);
  });

  it("fails an orphan class, a hollow rule and a runtime entry gone stale", () => {
    const [sheet] = treeOf([
      {
        path: join(dir, "onboarding-planted.css"),
        area: "src",
        text: ".ob-live { x: 1 } .ob-live-card { x: 1 } .is-error {} .is-picked { x: 1 }",
      },
    ]).files;
    if (!sheet) throw new Error("the planted sheet was not read");
    const findings = deadCss(
      [sheet],
      [{ path: join(dir, "screen.tsx"), code: '<div className="ob-live" />' }],
    );
    expect(findings).toEqual([
      "src/features/onboarding/onboarding-planted.css: .ob-live-card is named by no shipped source",
      "src/features/onboarding/onboarding-planted.css: .is-picked is named by no shipped source",
      "onboarding-gate.tsx is not one file under src/",
      "trust.tsx is not one file under src/",
      "src/features/onboarding/onboarding-planted.css: .is-error has an empty body",
    ]);
    expect(deadCss([], composers)).toEqual([]);
  });

  it("gates the onboarding sheets and the conversation folder only", () => {
    const sheet = (rel: string) => ({
      path: join(dir, rel),
      area: "src" as const,
      text: "",
    });
    const corpus = rule.corpus(
      treeOf([
        sheet("onboarding.css"),
        sheet("onboarding-conversation/review-deck.css"),
        sheet("other.css"),
        sheet("deeper/onboarding-x.css"),
      ]),
    );
    expect(corpus.map((file) => file.rel)).toEqual([
      "src/features/onboarding/onboarding.css",
      "src/features/onboarding/onboarding-conversation/review-deck.css",
    ]);
  });
});
