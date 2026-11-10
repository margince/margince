// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { findingsIn, rule } from "./controlheight.census";

// Each case is a shape the tree writes, and the one it must not report.
describe("what a second height looks like", () => {
  const corpus = new Set(["thing", "field"]);
  const found = (css: string) =>
    findingsIn("probe.css", css, corpus).map((one) => one.says);

  it("reads a fixed height, a floor, and their logical spellings", () => {
    expect(found(".thing { height: 26px; }")).toEqual(["height:26px"]);
    expect(found(".thing { min-height: 44px; }")).toEqual(["min-height:44px"]);
    expect(found(".thing { block-size: 2rem; }")).toEqual(["block-size:2rem"]);
    expect(found(".thing { min-block-size: var(--space-6); }")).toEqual([
      "min-block-size:var(--space-6)",
    ]);
  });

  it("passes the token itself, in any of the four spellings", () => {
    expect(found(".thing { min-block-size: var(--controlHeight); }")).toEqual(
      [],
    );
    expect(found(".thing { height: var(--controlHeight); }")).toEqual([]);
  });

  it("reads a rule the element itself selects", () => {
    expect(found(".bar button { height: 26px; }")).toEqual(["height:26px"]);
  });

  it("reads a rule through any one of its :is() alternatives", () => {
    expect(found(":is(.thing, .other) { height: 26px; }")).toEqual([
      "height:26px",
    ]);
    expect(found(".bar :where(.other, button) { height: 26px; }")).toEqual([
      "height:26px",
    ]);
  });

  it("reads a height inside a breakpoint, which is where a second one hides", () => {
    expect(
      found("@media (max-width: 700px) { .thing { min-height: 48px; } }"),
    ).toEqual(["min-height:48px"]);
  });

  it("files a rule against what it selects, not what it descends from", () => {
    // `.thing .mark` draws the mark, not the control around it.
    expect(found(".thing .mark { height: 14px; }")).toEqual([]);
    // And the class a `:not()` excuses is not the subject either.
    expect(found(".bar button:not(.field) { min-height: 44px; }")).toEqual([
      "min-height:44px",
    ]);
  });

  it("reads neither a ceiling nor a line", () => {
    expect(found(".thing { max-height: 60px; line-height: 20px; }")).toEqual(
      [],
    );
  });

  it("says nothing about a box nobody presses", () => {
    expect(found(".plate { height: 26px; }")).toEqual([]);
  });

  it("names the class and the selector, so ACCEPTED can be keyed by either", () => {
    const [one] = findingsIn(
      "probe.css",
      ".bar .thing { height: 26px; }",
      corpus,
    );
    expect(one.keys).toEqual(["thing", ".bar .thing"]);
  });
});

describe("the controlheight rule", () => {
  it("reports a planted second height through its own corpus and across", () => {
    const findings = plantedFindings(rule, [
      {
        path: join(srcDir, "features", "p.css"),
        area: "src",
        text: ".chip-go { cursor: pointer; height: 26px; }\n",
      },
    ]);
    expect(findings[0]).toBe("src/features/p.css:1 .chip-go — height:26px");
  });
});
