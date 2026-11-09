// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir } from "../tree";
import {
  cssFindings,
  inlineFindings,
  RESETS,
  rule,
  uppercasingFindings,
} from "./type-source.census";

describe("the gate sees what it claims to see", () => {
  const sheet = "src/screens/fixture.css";

  it.each([
    ["font-size", "0.875rem"],
    ["line-height", "1.25rem"],
    ["letter-spacing", "0.02em"],
    ["text-transform", "uppercase"],
  ])("refuses a %s by value and accepts inherit", (property, value) => {
    expect(cssFindings(sheet, `.a { ${property}: ${value}; }`)).toEqual([
      `${sheet}:1: ${property}: ${value}`,
    ]);
    expect(cssFindings(sheet, `.a { ${property}: inherit; }`)).toEqual([]);
  });

  it("lets a reset hand back the platform's casing, and nobody else", () => {
    const declaration = ".a { text-transform: none; }";
    expect(cssFindings(RESETS[0], declaration)).toEqual([]);
    expect(cssFindings(sheet, declaration)).toEqual([
      `${sheet}:1: text-transform: none`,
    ]);
  });

  it("lets a reset stop a footnote ordinal growing its line, and nobody else", () => {
    const neutralised = "sub,\nsup {\n  line-height: 0;\n}";
    expect(cssFindings(RESETS[0], neutralised)).toEqual([]);
    expect(cssFindings(RESETS[1], neutralised)).toEqual([]);
    // Same declaration, same file, a selector that is not sub/sup.
    expect(
      cssFindings(RESETS[0], ".pdigest-ref {\n  line-height: 0;\n}"),
    ).toEqual([`${RESETS[0]}:2: line-height: 0`]);
    // A third element added to the list does not come with it.
    expect(
      cssFindings(RESETS[0], "sub,\nsup,\nspan {\n  line-height: 0;\n}"),
    ).toEqual([`${RESETS[0]}:4: line-height: 0`]);
    // Same selector, a leading of somebody's choosing rather than none.
    expect(
      cssFindings(RESETS[0], "sub,\nsup {\n  line-height: 1.2;\n}"),
    ).toEqual([`${RESETS[0]}:3: line-height: 1.2`]);
    // Same declaration, same selector, a sheet that is not a reset.
    expect(cssFindings(sheet, neutralised)).toEqual([
      `${sheet}:3: line-height: 0`,
    ]);
    // And a rule nested in an @media reads its own selector, not the query's.
    expect(
      cssFindings(
        sheet,
        "@media print {\n  sub {\n    line-height: 0;\n  }\n}",
      ),
    ).toEqual([`${sheet}:3: line-height: 0`]);
  });

  it("takes a type token as a font shorthand and refuses a spelled one", () => {
    expect(cssFindings(sheet, ".a { font: var(--fontBody); }")).toEqual([]);
    expect(cssFindings(sheet, ".a { font: inherit; }")).toEqual([]);
    expect(
      cssFindings(sheet, ".a { font: 400 0.875rem/1.25rem Geist; }"),
    ).toEqual([`${sheet}:1: font: 400 0.875rem/1.25rem Geist`]);
  });

  it("takes a weight token and refuses a spelled weight", () => {
    expect(
      cssFindings(sheet, ".a { font-weight: var(--fontWeightBold); }"),
    ).toEqual([]);
    expect(cssFindings(sheet, ".a { font-weight: 700; }")).toEqual([
      `${sheet}:1: font-weight: 700`,
    ]);
    // A size token is not a weight: it would set the weight to a whole
    // shorthand and compute to nothing.
    expect(cssFindings(sheet, ".a { font-weight: var(--fontBody); }")).toEqual([
      `${sheet}:1: font-weight: var(--fontBody)`,
    ]);
  });

  it("reads past a comment without reading it", () => {
    const source = "/* font-size: 13px is wrong */\n.a { font: inherit; }";
    expect(cssFindings(sheet, source)).toEqual([]);
    expect(cssFindings(sheet, "/* a */\n.a { font-size: 13px; }")).toEqual([
      `${sheet}:2: font-size: 13px`,
    ]);
  });

  it("leaves font-family to the face gate", () => {
    expect(
      cssFindings(sheet, ".a { font-family: var(--fontFamilyMono); }"),
    ).toEqual([]);
  });

  const moduleFile = "src/screens/fixture.tsx";
  const uppercased = (text: string) =>
    uppercasingFindings(moduleFile, parseSource(moduleFile, text));

  it.each(["fontSize", "lineHeight", "letterSpacing", "textTransform"])(
    "refuses %s on a style object however it is spelled",
    (property) => {
      const source = `const s = { ${property}: "13px" };`;
      expect(inlineFindings(moduleFile, source)).toEqual([
        `${moduleFile}:1: ${property}: "13px"`,
      ]);
    },
  );

  it("takes a token on a style object and refuses a value", () => {
    expect(
      inlineFindings(moduleFile, 'const s = { font: "var(--fontBody)" };'),
    ).toEqual([]);
    expect(
      inlineFindings(moduleFile, 'const s = { font: "inherit" };'),
    ).toEqual([]);
    expect(
      // A template literal, so the interpolation under test stays text.
      inlineFindings(moduleFile, `const s = { font: "var(\${token})" };`),
    ).toEqual([]);
    expect(
      inlineFindings(moduleFile, 'const s = { font: "13px Geist" };'),
    ).toEqual([`${moduleFile}:1: font: "13px Geist"`]);
    expect(
      inlineFindings(
        moduleFile,
        'const s = { fontWeight: "var(--fontWeightBold)" };',
      ),
    ).toEqual([]);
    expect(
      inlineFindings(moduleFile, 'const s = { fontWeight: "700" };'),
    ).toEqual([`${moduleFile}:1: fontWeight: "700"`]);
  });

  it("catches a style object hoisted out of the element it dresses", () => {
    const source = [
      "const field: CSSProperties = {",
      '  padding: "var(--space-2)",',
      '  fontSize: "13px",',
      "};",
    ].join("\n");
    expect(inlineFindings(moduleFile, source)).toEqual([
      `${moduleFile}:3: fontSize: "13px"`,
    ]);
  });

  it("refuses small caps by every road that draws them", () => {
    for (const declaration of [
      "font-variant-caps: small-caps",
      "font-variant-caps: all-small-caps",
      "font-variant: small-caps",
      "font-variant: common-ligatures petite-caps",
      'font-feature-settings: "smcp" 1',
      'font-feature-settings: "c2sc", "smcp"',
      'font-feature-settings: "pcap"',
    ]) {
      expect(cssFindings(sheet, `.a { ${declaration}; }`), declaration).toEqual(
        [`${sheet}:1: ${declaration.replace(/:\s*/, ": ")}`],
      );
    }
  });

  it("leaves the rest of the font-variant family alone", () => {
    // Tabular figures matter most here: `.t-num` lines up columns of money
    // across the tree.
    for (const declaration of [
      "font-variant-numeric: tabular-nums",
      "font-variant-numeric: normal",
      "font-variant-caps: normal",
      "font-variant-caps: inherit",
      "font-variant: common-ligatures tabular-nums",
      'font-feature-settings: "tnum" 1',
      "font-feature-settings: normal",
    ]) {
      expect(cssFindings(sheet, `.a { ${declaration}; }`), declaration).toEqual(
        [],
      );
    }
  });

  it("leaves fontFamily to the face gate", () => {
    expect(
      inlineFindings(
        moduleFile,
        'const s = { fontFamily: "var(--fontFamilyMono)" };',
      ),
    ).toEqual([]);
  });

  it("catches an uppercased string wherever JSX renders it", () => {
    for (const body of [
      "<span>{name.toUpperCase()}</span>",
      "<span>{name.toLocaleUpperCase(locale)}</span>",
      "<abbr title={code.toUpperCase()} />",
      // Template literals, so the interpolation under test stays text.
      `<span>{\`\${code.toUpperCase()} · ok\`}</span>`,
      `<abbr title={\`\${code.toUpperCase()}\`} />`,
      "<span>{(name.toUpperCase())}</span>",
    ]) {
      const found = uppercased(`const a = ${body};`);
      expect(found.length, body).toBe(1);
      expect(found[0], body).toMatch(/^src\/screens\/fixture\.tsx:1: /);
    }
  });

  it("leaves an uppercased string nobody reads alone", () => {
    // The first is a comparison written inside JSX. The rule asks where the
    // result lands.
    for (const source of [
      'const a = <span>{code.toUpperCase() === want ? "y" : "n"}</span>;',
      "if (a.toUpperCase() === b) { run(); }",
      "const key = name.toUpperCase();",
      "seen.set(code.toUpperCase(), value);",
      "const sorted = rows.sort((a, b) => a.toUpperCase() < b.toUpperCase() ? -1 : 1);",
      "post({ currency: input.trim().toUpperCase() });",
    ]) {
      expect(uppercased(source), source).toEqual([]);
    }
  });
});

describe("the type-source rule", () => {
  it("reports planted type by value through its own corpus and visit", () => {
    expect(
      plantedFindings(rule, [
        {
          path: join(srcDir, "screens", "planted.css"),
          area: "src",
          text: ".a { font-size: 13px; }",
        },
        {
          path: join(srcDir, "screens", "planted.tsx"),
          area: "src",
          text: 'const a = <b style={{ fontWeight: "700" }}>{n.toUpperCase()}</b>;',
        },
        {
          path: join(srcDir, "design-system", "tokens.css"),
          area: "src",
          text: ".a { font-size: 13px; }",
        },
      ]),
    ).toEqual([
      "src/screens/planted.css:1: font-size: 13px",
      'src/screens/planted.tsx:1: fontWeight: "700"',
      "src/screens/planted.tsx:1: n.toUpperCase()",
    ]);
  });
});
