// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir, treeOf } from "../tree";
import {
  alignedIn,
  BASELINE,
  buttonClassesIn,
  faceVariantsIn,
  judged,
  ownClassesIn,
  rule,
  startsLabel,
} from "./button-align.census";

const classes = new Set(["btn", "pick-row"]);
const found = (css: string) =>
  alignedIn("probe.css", css, classes).map((one) => one.says);

describe("what a local start alignment on a Button looks like", () => {
  it("reads justify-content and text-align on a compound naming .btn", () => {
    expect(found(".menu .btn { justify-content: flex-start; }")).toEqual([
      "probe.css:1 .menu .btn { justify-content: flex-start }",
    ]);
    expect(found(".btn.x { text-align: left; }")).toEqual([
      "probe.css:1 .btn.x { text-align: left }",
    ]);
    expect(found(".x > .btn { justify-content: start; }")).toHaveLength(1);
  });

  it("reads the spellings that say the same thing", () => {
    expect(startsLabel("justify-content", "safe flex-start")).toBe(true);
    expect(startsLabel("place-content", "center start")).toBe(true);
    expect(startsLabel("place-content", "start")).toBe(true);
    expect(startsLabel("text-align", "start !important")).toBe(true);
    expect(startsLabel("justify-content", "space-between")).toBe(false);
    expect(startsLabel("text-align", "center")).toBe(false);
    expect(startsLabel("inset-inline-start", "0")).toBe(false);
  });

  it("reads .btn through :is(), :where(), a selector list and nesting", () => {
    expect(found(":is(.btn, .chip) { text-align: start; }")).toEqual([
      "probe.css:1 :is(.btn, .chip) { text-align: start }",
    ]);
    expect(found(".x :where(.y .btn) { text-align: start; }")).toHaveLength(1);
    expect(found(".a, .x .btn { text-align: start; }")).toEqual([
      "probe.css:1 .x .btn { text-align: start }",
    ]);
    expect(found(".card { & .btn { justify-content: flex-start; } }")).toEqual([
      "probe.css:1 .card .btn { justify-content: flex-start }",
    ]);
  });

  it("reads a rule inside a breakpoint", () => {
    expect(
      found("@media (max-width: 40rem) { .x .btn { text-align: left; } }"),
    ).toHaveLength(1);
  });

  it("reads a class the markup hands a Button, the button element and a class attribute", () => {
    expect(found(".pick-row { justify-content: flex-start; }")).toHaveLength(1);
    expect(found(".list button { text-align: left; }")).toHaveLength(1);
    expect(found('.x [class~="btn"] { text-align: start; }')).toHaveLength(1);
  });

  it("reads a descendant of a Button, which aligns the label inside it", () => {
    expect(found(".btn .label { text-align: start; }")).toHaveLength(1);
  });

  it("passes what does not select a Button", () => {
    expect(found(".btn + .hint { text-align: start; }")).toEqual([]);
    expect(found(".btn ~ p { text-align: left; }")).toEqual([]);
    expect(found(".row:has(.btn) { justify-content: flex-start; }")).toEqual(
      [],
    );
    expect(found(".x:not(.btn) { text-align: left; }")).toEqual([]);
    expect(found(".x:not([class~=btn]) { text-align: left; }")).toEqual([]);
    expect(found(".btn-shell { text-align: start; }")).toEqual([]);
    expect(found(".buttons { text-align: start; }")).toEqual([]);
    expect(found(".btn { inset-inline-start: 0; }")).toEqual([]);
    expect(found(".btn { justify-content: space-between; }")).toEqual([]);
  });
});

describe("the classes on a Button", () => {
  const buttonPath = join(srcDir, "design-system", "button.tsx");
  const button = parseSource(buttonPath, readFileSync(buttonPath, "utf8"));

  it("reads className on <Button> and not on another element", () => {
    const source = parseSource(
      "probe.tsx",
      `const a = <Button className="pick-row">x</Button>;
       const b = <span className="note">y</span>;`,
    );
    const read = buttonClassesIn([
      { path: "probe.tsx", rel: "probe.tsx", area: "src", text: "", source },
    ]);
    expect([...read]).toEqual(["pick-row"]);
  });

  it("reads every variant faceClass takes, through a type alias", () => {
    const source = parseSource(
      "probe.tsx",
      `type Variant = "primary" | "link";
       export function faceClass(variant: Variant | "bare") { return ""; }`,
    );
    expect(faceVariantsIn(source)).toEqual(["primary", "link", "bare"]);
  });

  it("reads the classes Button renders on its <button>, faces included", () => {
    const own = new Set(ownClassesIn(button));
    for (const name of ["btn", "btn-icon", "btn-ghost", "btn-aiQuiet"]) {
      expect(own).toContain(name);
    }
    for (const name of ["btn-link", "btn-danger", "btn-bare"]) {
      expect(own).toContain(name);
    }
    for (const name of ["btn-shell", "btn-with-reason", "link-button"]) {
      expect(own).not.toContain(name);
    }
  });

  it("finds a start alignment on a compound naming a face class", () => {
    const fromButton = buttonClassesIn([
      {
        path: buttonPath,
        rel: "src/design-system/button.tsx",
        area: "src",
        text: "",
        source: button,
      },
    ]);
    const on = (css: string) => alignedIn("probe.css", css, fromButton);
    expect(on(".x .btn-link { text-align: start; }")).toHaveLength(1);
    expect(on(".x .btn-aiQuiet { justify-content: flex-start; }")).toHaveLength(
      1,
    );
    expect(on(".x .btn-shell { text-align: start; }")).toEqual([]);
    expect(on(".x .link-button { text-align: start; }")).toEqual([]);
  });
});

describe("the baseline", () => {
  const one = {
    key: "a.css .x .btn text-align",
    says: "a.css:3 .x .btn { text-align: start }",
    left: false,
  };

  it("excuses a listed rule and reports an unlisted one", () => {
    expect(judged([one], new Map([[one.key, "why"]]))).toEqual([]);
    expect(judged([one], new Map())).toEqual([one.says]);
  });

  it("excuses no `left`, which is the wrong edge right to left", () => {
    const left = {
      ...one,
      says: "a.css:3 .x .btn { text-align: left }",
      left: true,
    };
    expect(judged([left], new Map([[one.key, "why"]]))).toEqual([left.says]);
  });

  it("excuses one declaration per entry, so a second one under its key is a finding", () => {
    const again = { ...one, says: "a.css:9 .x .btn { text-align: start }" };
    expect(judged([one, again], new Map([[one.key, "why"]]))).toEqual([
      again.says,
    ]);
  });

  it("keys a declaration by sheet, selector and property", () => {
    const [aligned] = alignedIn(
      "a.css",
      ".x .btn { text-align: start; }",
      classes,
    );
    expect(aligned.key).toBe("a.css .x .btn text-align");
  });

  it("only shrinks", () => {
    expect(
      BASELINE.size,
      'Give the Button align="start" instead of a new BASELINE entry. When you delete one, lower this number.',
    ).toBe(9);
  });

  it("reports an entry no rule matches any more", () => {
    expect(judged([], new Map([[one.key, "why"]]))).toEqual([
      "BASELINE excuses a.css .x .btn text-align, which no rule start-aligns any more; delete the line",
    ]);
  });
});

describe("the census over a planted tree", () => {
  const unbaselined = (findings: readonly string[]) =>
    findings.filter((line) => !line.startsWith("BASELINE excuses"));

  it("reads every sheet, a Button class from markup, and skips base.css", () => {
    const findings = plantedFindings(rule, [
      {
        path: join(srcDir, "design-system", "base.css"),
        area: "src",
        text: '.btn[data-align="start"] { justify-content: flex-start; }',
      },
      {
        path: join(srcDir, "features", "x", "x.css"),
        area: "src",
        text: ".x-row { text-align: left; } .x-note { text-align: left; }",
      },
      {
        path: join(srcDir, "features", "x", "x.tsx"),
        area: "src",
        text: 'export const X = () => <Button className="x-row">Go</Button>;',
      },
      {
        path: join(srcDir, "features", "x", "x.stories.tsx"),
        area: "src",
        text: 'export const S = () => <Button className="x-note">Go</Button>;',
      },
    ]);
    expect(unbaselined(findings)).toEqual([
      "src/features/x/x.css:1 .x-row { text-align: left }",
    ]);
  });
});

describe("the floors", () => {
  it("fail a corpus too small to judge", () => {
    const tree = treeOf([
      {
        path: join(srcDir, "design-system", "base.css"),
        area: "src",
        text: ".btn { justify-content: center; }",
      },
      {
        path: join(srcDir, "features", "x", "x.tsx"),
        area: "src",
        text: 'export const X = () => <Button className="x-row">Go</Button>;',
      },
    ]);
    expect(rule.floors?.(rule.corpus(tree), tree)).toEqual([
      "fewer than 100 stylesheets were read",
      "fewer than 2000 CSS rules were read",
      "button.tsx no longer reads as the btn class and faceClass's seven or more variants",
      "30 or fewer classes were read off a <Button>",
      "base.css's own align rule no longer reads as start alignment, so the reader is blind",
    ]);
  });
});
