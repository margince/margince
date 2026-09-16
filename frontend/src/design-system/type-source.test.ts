// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { filesMatching, parseSource } from "../../scripts/lib/source-tree";

// ONE source for type. Outside tokens.css nothing in this tree declares a size,
// a leading, a weight or a tracking BY VALUE: a rule wears a `--font*` token or
// it inherits, and there is no third answer.
//
// The rule is old and the gate is not, which is the whole problem it solves.
// Sizes were the first thing this tree spelled twice — a 13px here, a 0.8125rem
// there, a `line-height: 1.4` beside a token that already said 1.25rem — and
// none of it fails anything. It renders. A reader sees one screen a hair off
// another and cannot say which is wrong, because both are: the token moved and
// the hand-typed copy did not. A weight is worse still, because a value the
// request never loaded is not refused but SYNTHESIZED (weights.test.ts).
//
// So the obligation is derived rather than listed: the corpus is every
// stylesheet and every non-test module under src/, and the check is on the
// DECLARATION, not on a list of files anyone maintains. A gate that reads a
// smaller tree than it claims reports PASS and has no failing assertion to
// notice it, so the corpus guards itself below before a single rule runs.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const read = (file: string) => readFileSync(join(frontendRoot, file), "utf8");

// The one file that may declare type by value. It IS the source.
const SOURCE = "src/design-system/tokens.css";

// The two UA resets, and the only reason they are named. Two declarations in
// them are LAYOUT FACTS about what a browser already did, not type anybody
// chose, and each is allowed only in these files and only in the shape below:
//
//   text-transform: none   hands a control back the casing the platform
//                          stylesheet took from it
//   line-height: 0         on `sub`/`sup` alone, so a footnote ordinal does not
//                          grow the line it hangs off — `<sup
//                          className="pdigest-ref">` in the onboarding digest
//                          is what needs it
//
// Everywhere else either declaration is somebody restyling text, which is what
// this refuses. The exceptions are kept this narrow on purpose: an allowlist by
// FILE would let any type declaration through a reset, and a reset is the one
// sheet every element in the product inherits from.
const RESETS: readonly string[] = ["src/app.css", "src/mcp-apps/view.css"];

// The rule a `line-height: 0` may stand on: a selector list of nothing but
// `sub` and `sup`. Adding a third element to that list is adding a third
// element to the exception, so it has to be written here to take effect.
const NEUTRALISED = /^(sub|sup)(\s*,\s*(sub|sup))*$/;

// A token reference, which is the only value that is not a second spelling.
// `--font` plus a capital, so `--fontBody`, `--fontHeadingLarge` and
// `--fontWeightMedium` all read as one shape and `--font-size-13` does not.
const TYPE_TOKEN = /^var\(\s*--font[A-Z][A-Za-z]*\s*\)$/;
const WEIGHT_TOKEN = /^var\(\s*--fontWeight[A-Z][A-Za-z]*\s*\)$/;

// `inherit` is the reset's answer on BOTH sides of the wire — a `font: inherit`
// in a stylesheet and a `font: "inherit"` on a style object are the same
// declaration doing the same job, so they are one rule and not two.
const INHERIT = "inherit";

// Longest property first, so `font-size` is tried before `font` at the same
// position. Two absences are deliberate. `font-family` is mono.test.ts's
// question, and a family is not a size. `font-variant-numeric` is not listed at
// all, which is what ALLOWS it: `.t-num`'s tabular figures are an alignment
// fact about a column of money, not a decision about how loud text is, and the
// tree draws it about seventy times.
const CSS_DECLARATION =
  /(?:^|[;{}\s])(font-feature-settings|font-variant-caps|font-variant|font-size|font-weight|line-height|letter-spacing|text-transform|font)\s*:\s*([^;}]*)/g;

// Small caps are UPPERCASE by another road. `text-transform` is the spelling
// this gate has always refused, and refusing only that leaves three ways to
// draw the same shouted label — the caps keyword on `font-variant-caps`, the
// same keyword inside the `font-variant` shorthand, and the OpenType feature
// turned on by hand. All four are one decision, so they answer to one rule.
const CAPS_KEYWORD =
  /\b(small-caps|all-small-caps|petite-caps|all-petite-caps|unicase|titling-caps)\b/;
// The features themselves: smcp is small caps, c2sc caps-to-small-caps, pcap
// petite. Quoted, because that is the only way the property takes a tag.
const CAPS_FEATURE = /["'](smcp|c2sc|pcap)["']/;

// A style object's property. Matches wherever the object is written — inline in
// a `style={{ … }}`, or hoisted to a `const … : CSSProperties`, which is the
// same declaration one name away and must not be the way around the gate.
// The value runs to the next comma or brace — but an interpolation carries a
// brace of its own, so `${…}` is consumed whole rather than ending the value
// one character into it.
const INLINE_DECLARATION =
  /(?:^|[{,\s])(fontSize|lineHeight|letterSpacing|textTransform|fontWeight|font)\s*:\s*((?:\$\{[^}]*\}|[^,}\n])*)/g;

/** Comments blanked, so a sentence ABOUT a declaration is not one — and every
 * line still counts to the line a reader can open. */
const withoutComments = (source: string) =>
  source.replace(/\/\*[\s\S]*?\*\//g, (comment) =>
    comment.replace(/[^\n]/g, " "),
  );

const lineOf = (text: string, index: number) =>
  text.slice(0, index).split("\n").length;

/**
 * The selector list of the rule a declaration stands in: the text between the
 * brace it opens under and whatever ended the statement before it. A nested
 * rule reads its own selector rather than the `@media` around it, which is what
 * keeps the exception below from widening inside one.
 */
function selectorOf(text: string, index: number): string {
  const open = text.lastIndexOf("{", index);
  if (open < 0) return "";
  const before = text.slice(0, open);
  const ended = Math.max(
    before.lastIndexOf("}"),
    before.lastIndexOf("{"),
    before.lastIndexOf(";"),
  );
  return before
    .slice(ended + 1)
    .replace(/\s+/g, " ")
    .trim();
}

/** A style object's value with its quoting removed: `"inherit"` is `inherit`,
 * and a template literal is whatever it spells. */
const unquoted = (value: string) =>
  value
    .trim()
    .replace(/^["'`]/, "")
    .replace(/["'`]$/, "");

/**
 * Whether a CSS declaration is allowed to say what it says.
 *
 * `inherit` passes everywhere: it declares no value, it defers to one. A weight
 * takes a weight token and a `font` shorthand takes any type token; the other
 * four take nothing else at all, because there is no token for a bare size,
 * leading or tracking — those exist only INSIDE a `font` shorthand, which is
 * what makes a level impossible to half-wear.
 */
function cssDeclarationHolds(
  file: string,
  selector: string,
  property: string,
  value: string,
): boolean {
  if (value === INHERIT) return true;
  if (property === "font") return TYPE_TOKEN.test(value);
  if (property === "font-weight") return WEIGHT_TOKEN.test(value);
  // The caps family: each is allowed to say anything EXCEPT draw capitals.
  // `font-variant` and `font-feature-settings` carry other things too — figure
  // shapes, ligatures, kerning — and those are not this gate's business.
  if (property === "font-variant-caps") return value === "normal";
  if (property === "font-variant") return !CAPS_KEYWORD.test(value);
  if (property === "font-feature-settings") return !CAPS_FEATURE.test(value);
  if (!RESETS.includes(file)) return false;
  if (property === "text-transform") return value === "none";
  if (property === "line-height") {
    return value === "0" && NEUTRALISED.test(selector);
  }
  return false;
}

/** Whether a style object's declaration is allowed to say what it says. */
function inlineDeclarationHolds(property: string, value: string): boolean {
  if (value === INHERIT) return true;
  if (property === "font")
    return TYPE_TOKEN.test(value) || isComputedToken(value);
  if (property === "fontWeight") return WEIGHT_TOKEN.test(value);
  // fontSize, lineHeight, letterSpacing and textTransform have no spelling a
  // style object may use: the token is a `font` shorthand and nothing smaller.
  return false;
}

/**
 * A `var()` whose property name is interpolated — `var(${token})`. It resolves
 * to a custom property or to nothing, and a custom property is a token by
 * construction, so this is a reference and not a second spelling. The type
 * specimen page is the caller: it walks its own list of tokens and renders each
 * one, which is the one place a token name is data.
 */
const isComputedToken = (value: string) =>
  /^var\(\s*\$\{[^}]+\}\s*\)$/.test(value);

/** Every declaration in one stylesheet that spells type by value. */
function cssFindings(file: string, source: string): string[] {
  const text = withoutComments(source);
  return [...text.matchAll(CSS_DECLARATION)].flatMap((match) => {
    const [property, value] = [match[1], match[2].trim()];
    const selector = selectorOf(text, match.index);
    if (cssDeclarationHolds(file, selector, property, value)) return [];
    return [`${file}:${lineOf(text, match.index)}: ${property}: ${value}`];
  });
}

/** Every style-object declaration in one module that spells type by value. */
function inlineFindings(file: string, source: string): string[] {
  return [...source.matchAll(INLINE_DECLARATION)].flatMap((match) => {
    const [property, raw] = [match[1], match[2]];
    const value = unquoted(raw);
    if (inlineDeclarationHolds(property, value)) return [];
    return [
      `${file}:${lineOf(source, match.index)}: ${property}: ${raw.trim()}`,
    ];
  });
}

const UPPERCASING = new Set(["toUpperCase", "toLocaleUpperCase"]);

/**
 * Whether an expression stands where JSX RENDERS it: as a child, as an
 * attribute value, or as a piece of a template literal in either place.
 *
 * The distinction is the whole arm. `code.toUpperCase()` compared against
 * another string, used as a map key, or normalising a currency before it is
 * sent to the server is data handling, and none of it is visible; the same call
 * dropped between two tags is a casing decision taken at the call site, which
 * is what `text-transform` was refused for. A rule that could not tell them
 * apart would have to refuse the language's own uppercase function.
 *
 * Parentheses and template spans are climbed through because they change where
 * the call is WRITTEN and not where its result lands. A binary expression is
 * deliberately not: `a.toUpperCase() === b` renders nothing, and a
 * concatenation would need its own decision rather than this one by default.
 */
function isRendered(call: ts.Node): boolean {
  let here: ts.Node = call;
  let parent: ts.Node | undefined = here.parent;
  while (
    parent &&
    (ts.isParenthesizedExpression(parent) ||
      ts.isTemplateSpan(parent) ||
      ts.isTemplateExpression(parent))
  ) {
    here = parent;
    parent = here.parent;
  }
  if (!parent || !ts.isJsxExpression(parent)) return false;
  const holder = parent.parent;
  return (
    holder !== undefined &&
    (ts.isJsxAttribute(holder) ||
      ts.isJsxElement(holder) ||
      ts.isJsxFragment(holder) ||
      ts.isJsxSelfClosingElement(holder))
  );
}

/** Every uppercased string one module puts on the screen. */
function uppercasingFindings(file: string, source: string): string[] {
  const tree = parseSource(file, source);
  const found: string[] = [];
  const visit = (node: ts.Node): void => {
    if (
      ts.isCallExpression(node) &&
      ts.isPropertyAccessExpression(node.expression) &&
      UPPERCASING.has(node.expression.name.text) &&
      isRendered(node)
    ) {
      const { line } = tree.getLineAndCharacterOfPosition(node.getStart(tree));
      found.push(
        `${file}:${line + 1}: ${node.getText(tree).replace(/\s+/g, " ")}`,
      );
    }
    ts.forEachChild(node, visit);
  };
  visit(tree);
  return found;
}

const under = (pattern: RegExp) =>
  filesMatching(join(frontendRoot, "src"), pattern).map((file) =>
    relative(frontendRoot, file),
  );

describe("type comes from tokens.css and from nowhere else", () => {
  const stylesheets = under(/\.css$/);
  const modules = under(/\.tsx?$/).filter(
    (file) => !/\.test\.tsx?$/.test(file),
  );

  // Under-recognition is the one failure with no assertion to notice it: a walk
  // that returned four files would report a clean tree in the same word a real
  // pass uses. So the corpus is asserted before anything is read off it.
  it("reads the tree it claims", () => {
    expect(stylesheets).toContain("src/app.css");
    expect(stylesheets).toContain(SOURCE);
    expect(stylesheets.length).toBeGreaterThanOrEqual(100);
    // The module half has its own floor, and its own proof that the test
    // filter subtracts rather than empties: the design system alone is well
    // past this, and a walk that returned only tests would return none.
    expect(modules).toContain("src/design-system/provider-mark.tsx");
    expect(modules.length).toBeGreaterThanOrEqual(100);
  });

  it("declares no size, leading, weight or tracking by value", () => {
    const found = stylesheets
      .filter((file) => file !== SOURCE)
      .flatMap((file) => cssFindings(file, read(file)));
    expect(found, found.join("\n")).toEqual([]);
  });

  it("puts no type on a style object", () => {
    const found = modules.flatMap((file) => inlineFindings(file, read(file)));
    expect(found, found.join("\n")).toEqual([]);
  });

  // Only .tsx, and that is completeness rather than a shortcut: JSX is decided
  // by script kind, so a .ts file cannot hold the thing this looks for.
  it("renders no string it uppercased on the way to the screen", () => {
    const components = modules.filter((file) => file.endsWith(".tsx"));
    expect(components.length).toBeGreaterThanOrEqual(100);
    const found = components.flatMap((file) =>
      uppercasingFindings(file, read(file)),
    );
    expect(found, found.join("\n")).toEqual([]);
  });
});

// One planted pair per rule. A gate that has only ever seen a clean tree is a
// gate nobody has watched fail, and the shape it cannot see is the shape that
// ships.
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
    // A third element smuggled into the list does not come with it.
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
    // A size token is not a weight: the shorthand carries three other parts,
    // and `font-weight: var(--fontBody)` would set the weight to the whole
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
      // A template literal, so the interpolation this fixture is ABOUT stays
      // text rather than being read as one.
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
    // Tabular figures are the negative case that matters: `.t-num` is drawn
    // about seventy times in this tree, and a rule that swept the whole
    // font-variant family would refuse a column of money lining up.
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
      // Template literals, so the interpolation these fixtures are ABOUT stays
      // text rather than being read as one.
      `<span>{\`\${code.toUpperCase()} · ok\`}</span>`,
      `<abbr title={\`\${code.toUpperCase()}\`} />`,
      "<span>{(name.toUpperCase())}</span>",
    ]) {
      const found = uppercasingFindings(moduleFile, `const a = ${body};`);
      expect(found.length, body).toBe(1);
      expect(found[0], body).toMatch(/^src\/screens\/fixture\.tsx:1: /);
    }
  });

  it("leaves an uppercased string nobody reads alone", () => {
    // The sharp one is the first: a comparison written INSIDE JSX is still a
    // comparison, and the rule is about where the RESULT lands.
    for (const source of [
      'const a = <span>{code.toUpperCase() === want ? "y" : "n"}</span>;',
      "if (a.toUpperCase() === b) { run(); }",
      "const key = name.toUpperCase();",
      "seen.set(code.toUpperCase(), value);",
      "const sorted = rows.sort((a, b) => a.toUpperCase() < b.toUpperCase() ? -1 : 1);",
      "post({ currency: input.trim().toUpperCase() });",
    ]) {
      expect(uppercasingFindings(moduleFile, source), source).toEqual([]);
    }
  });
});
