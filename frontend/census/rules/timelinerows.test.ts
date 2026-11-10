// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { judge, parsedReach, rule, writtenReach } from "./timelinerows.census";

describe("a timeline row rule", () => {
  it.each([
    [".timeline li", [".timeline li"]],
    [
      ".timeline.timeline-plain li + li",
      [".timeline.timeline-plain li + li", ".timeline.timeline-plain li + li"],
    ],
    [".timeline .x li", [".timeline .x li"]],
    [
      ".timeline .tl-thread-messages > li",
      [".timeline .tl-thread-messages > li"],
    ],
    [":is(.timeline, .other) li", [".timeline li"]],
  ])(
    "refuses %s, which also reaches li items nested inside a row",
    (selector, leaks) => {
      expect(judge(selector).leaks).toEqual(leaks);
    },
  );

  it.each([
    ".timeline > li",
    ".timeline.timeline-plain > li + li",
    ".timeline > li:first-child > .tl-rail::before",
    ".timeline-header li",
  ])("accepts %s", (selector) => {
    expect(judge(selector).leaks).toEqual([]);
  });

  it("counts a selector the parser misses as a census shortfall", () => {
    const missed = ".timeline > *|li";
    expect(writtenReach(`${missed} { color: red; }`)).toBe(1);
    expect(parsedReach([missed])).toBe(0);
  });

  it("reports a planted leak through its own corpus and across", () => {
    expect(
      plantedFindings(rule, [
        {
          path: join(srcDir, "features", "planted.css"),
          area: "src",
          text: ".timeline li { color: red; }\n.timeline > li { color: blue; }",
        },
      ]),
    ).toEqual(["src/features/planted.css: .timeline li"]);
  });
});
