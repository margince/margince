// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { rulesIn } from "../../src/testing/css";
import { plantedFindings, srcDir } from "../tree";
import {
  fullRungOf,
  handRolled,
  markupIn,
  ownDeclarations,
  restyles,
  rule,
} from "./badge-spelling.census";

// The detector must see each shape, or a clean tree and a broken reader look
// the same.
describe("the detector", () => {
  const rung = fullRungOf(
    readFileSync(join(srcDir, "design-system", "tokens.css"), "utf8"),
  );
  const restyledIn = (css: string) =>
    rulesIn(css)
      .flatMap(restyles)
      .map(
        ({ selector, declaration }) => `${selector} ${declaration.property}`,
      );
  const pillsIn = (css: string) =>
    handRolled(
      rulesIn(css).map((rule) => ({ where: "x.css", rule })),
      rung,
    ).map((pill) => pill.family);
  const markup = (text: string, shipped = true) =>
    markupIn(parseSource("x.tsx", text), shipped).map((f) => f.says);

  it("reads a colour on a descendant badge, and not its margin", () => {
    expect(restyledIn(".x .badge { color: var(--danger); }")).toEqual([
      ".x .badge color",
    ]);
    expect(
      restyledIn(
        ".x .badge { margin-inline-start: var(--space-2); align-self: center; }",
      ),
    ).toEqual([]);
  });

  it("reads a child-combinator tone class and a compound", () => {
    expect(
      restyledIn(".x > .badge-warning { text-transform: uppercase; }"),
    ).toEqual([".x > .badge-warning text-transform"]);
    expect(restyledIn(".badge.x { letter-spacing: 0.04em; }")).toEqual([
      ".badge.x letter-spacing",
    ]);
  });

  it("reads a badge as an ancestor, and a property nobody listed", () => {
    expect(restyledIn(".badge svg { opacity: 0.6; }")).toEqual([
      ".badge svg opacity",
    ]);
    expect(restyledIn(".x .badge { --badge-ink: red; }")).toEqual([
      ".x .badge --badge-ink",
    ]);
  });

  it("reads a badge named through :where() or a class attribute", () => {
    expect(
      restyledIn(
        ':where(.x .badge) { color: red; } .y [class*="badge-"] { color: red; }',
      ),
    ).toEqual([":where(.x .badge) color", '.y [class*="badge-"] color']);
  });

  it("does not read .badges, a :not(.badge), or a :has(.badge)", () => {
    expect(
      restyledIn(
        ".x-badges { color: red; } .x > :not(.badge) { color: red; } .row:has(.badge) { background: red; }",
      ),
    ).toEqual([]);
  });

  it("reads a restyle inside @media, @supports and a nested block", () => {
    expect(
      restyledIn(
        "@media (max-width: 600px) { .x .badge { padding: 0; } }\n" +
          "@supports (display: grid) { .y .badge-ai { font-size: 1em; } }\n" +
          ".z { display: grid; .badge { border: 1px solid; } &.w > .badge-danger { background: red; } }",
      ),
    ).toEqual([
      ".x .badge padding",
      ".y .badge-ai font-size",
      ".z .badge border",
      ".z.w > .badge-danger background",
    ]);
  });

  it("reads one member of a selector list", () => {
    expect(restyledIn(".a, .b .badge, .c { color: red; }")).toEqual([
      ".b .badge color",
    ]);
  });

  it("reads a filled full-corner pill class, and not an unfilled one", () => {
    expect(
      pillsIn(
        ".foo-pill { background: var(--bgChip); border-radius: var(--r-full); }",
      ),
    ).toEqual(["foo-pill"]);
    expect(pillsIn(".foo-pill { border-radius: var(--r-full); }")).toEqual([]);
    expect(
      pillsIn(
        ".foo-pill { background: transparent; border-radius: var(--r-full); }",
      ),
    ).toEqual([]);
    expect(
      pillsIn(
        ".foo-tag { background: var(--bgChip); border-radius: var(--r-sm); }",
      ),
    ).toEqual([]);
  });

  it("reads a pill split across its base and a modifier, in a breakpoint", () => {
    expect(
      pillsIn(
        ".lead-tag { border-radius: 50%; }\n@media (min-width: 1px) { .lead-tag-warning { background-color: var(--warningBg); } }",
      ),
    ).toEqual(["lead-tag"]);
    expect(
      pillsIn(
        `.x-lozenge { border-radius: ${rung} ${rung}; background: var(--bgChip); }`,
      ),
    ).toEqual(["x-lozenge"]);
  });

  it("does not read a one-word control name, or a pill that is only an ancestor", () => {
    expect(
      pillsIn(
        ".filterpill { background: var(--bgElevated); border-radius: var(--r-full); }\n" +
          ".x-pill .dot { background: red; border-radius: 50%; }",
      ),
    ).toEqual([]);
  });

  it("reads a waiver on the declaration's line or above it, with a reason", () => {
    const [rule] = rulesIn(
      ".x .badge {\n  /* ds:ignore the host page draws this */\n  color: red;\n" +
        "  margin: 0; /* ds:ignore */\n  padding: 0;\n}",
    );
    expect(
      ownDeclarations(rule).map((d) => [d.property, d.line, d.waiver]),
    ).toEqual([
      ["color", 3, "reasoned"],
      ["margin", 4, "bare"],
      ["padding", 5, "none"],
    ]);
  });

  it("reads the badge class in className, however it is assembled", () => {
    expect(markup('<span className="badge badge-warning">x</span>')).toEqual([
      "<span> carries the badge class",
    ]);
    expect(markup('<span className={cx("badge", on && "x")}>x</span>')).toEqual(
      ["<span> carries the badge class"],
    );
    // biome-ignore lint/suspicious/noTemplateCurlyInString: a fixture of source code
    expect(markup("<b className={`x badge-${tone}`}>x</b>")).toEqual([
      "<b> carries the badge class",
    ]);
    expect(markup('<span className="x-badges tagpill">x</span>')).toEqual([]);
  });

  it("reads a class list assembled away from the element, in a shipped module only", () => {
    expect(markup('const classes = ["badge", "badge-ai"];')).toHaveLength(2);
    expect(markup('expect(el).toHaveClass("badge-ai");', false)).toEqual([]);
    expect(markup('const words = "A badge is a label.";')).toEqual([]);
  });

  it("reads a style or a class handed to Badge, under an alias too", () => {
    expect(markup("<Badge style={{ color: 'red' }}>x</Badge>")).toEqual([
      "<Badge> is handed style",
    ]);
    expect(
      markup(
        'import { Badge as Pill } from "../design-system/atoms";\n<Pill {...{ className: "x" }}>x</Pill>',
      ),
    ).toEqual(["<Badge> is handed {...{ className }}"]);
    expect(markup('<Badge tone="warning">x</Badge>')).toEqual([]);
  });
});

describe("the badge-spelling rule", () => {
  it("reports planted offences through its own corpus and across", () => {
    const tokens = join(srcDir, "design-system", "tokens.css");
    const findings = plantedFindings(rule, [
      { path: tokens, area: "src", text: ":root { --r-full: 999px; }" },
      {
        path: join(srcDir, "features", "planted.css"),
        area: "src",
        text: ".x .badge { color: red; }",
      },
      {
        path: join(srcDir, "features", "planted.tsx"),
        area: "src",
        text: 'export const F = () => <span className="badge">x</span>;\n',
      },
    ]);
    expect(findings).toEqual([
      "restyle: src/features/planted.css:1  .x .badge  { color: red }",
      "markup: src/features/planted.tsx:1  <span> carries the badge class",
    ]);
  });
});
