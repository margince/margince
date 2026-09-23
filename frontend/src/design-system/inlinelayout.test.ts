// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  extensionLayers,
  filesUnder,
  MODULE_FILE,
  parseSource,
  sourceFileAt,
} from "../../scripts/lib/source-tree";

// LAYOUT IS A CLASS, NOT A STYLE ATTRIBUTE, AND THE COUNT IS PINNED.
//
// The values are already tokens — `check-ds-spacing.sh` and `type-source.test.ts`
// see to that — so what is left is a rule nobody can restyle: a margin written
// at a call site belongs to that one element, and the next author who needs the
// same step writes it again a line further down, so a spacing decision has as
// many homes as there are elements wearing it. A class is one home, and
// `Stack` / `Row` are the same answer for a unit that has no stylesheet.
//
// ONLY A STATIC VALUE IS A RULE. An identifier, a call, a template with a
// substitution, a conditional or a member access is a number the render
// COMPUTED — a popover's resolved position, a bar filled to a percentage, a
// column width read off the table's own config — and no stylesheet can hold it.
//
// A WRAPPER IS NOT AN EXEMPTION, AND NEITHER IS A ROAD. The rule counts
// wherever the element can reach it: through a ternary, a guard, a cast, a
// spread, a nullish default, a style hoisted to a name of its own, a props
// object spread onto the element, or the `createElement` call JSX compiles to.
// A reader of the rendered page cannot tell those apart, and a census that
// could is one an author steps around by adding a `?:`.
//
// The width/height family IS in scope, measured on its own before it was let
// in: a static dimension is a rule a class holds as readily as a margin. The
// data case the family is usually excused for — a column width off the table's
// config — is already exempt one rule up, so excusing it twice would forgive
// nothing but the hand-written ones.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const repoRoot = join(frontendRoot, "..");
const sourceRoot = join(frontendRoot, "src");
const extensionsRoot = join(repoRoot, "extensions");

/**
 * A style property that lays something out, by family: the box's own space,
 * how it flows, and where it sits. Matched as a PREFIX, so `marginInlineStart`,
 * `gridTemplateColumns` and `overflowX` are covered without a list that goes
 * stale the day the platform ships another one. The logical sizes name
 * themselves, because `inlineSize` shares no prefix with the `width` it is.
 *
 * `transform` and `translate` are here because a static one MOVES the box;
 * `rotate`, `scale` and `opacity` are not, because they change what a box
 * looks like without changing where anything sits.
 */
const LAYOUT_PROPERTY =
  /^(margin|padding|gap|rowGap|columnGap|display|flex|grid|justify|align|place|inset|top|right|bottom|left|width|minWidth|maxWidth|height|minHeight|maxHeight|position|overflow|float|order|boxSizing|aspectRatio|transform|translate|zIndex|inlineSize|blockSize|minInlineSize|maxInlineSize|minBlockSize|maxBlockSize)/;

/** A type annotation that says an object IS a style, wherever it is written. */
const STYLE_ANNOTATION = /\bCSSProperties\b/;

/** What the fix is, said the same way at every finding. */
const FIX =
  "a class in the screen's sheet, or `Stack`/`Row` from design-system/stack.tsx";

/**
 * How many inline layout rules each file carries. The entry is EXACT in both
 * directions: a file over it fails naming the property, and a file under it
 * fails until the entry is lowered in the same change, because a baseline left
 * high is a budget the next author spends without deciding to.
 *
 * A file, not a line: a line number moves whenever anything above it is edited,
 * and a baseline that churns on unrelated changes is one contacts regenerate
 * without reading. Inline rather than in a JSON file beside it, the way
 * `cardzones.test.ts` keeps its allowlist — a number a reader meets in the same
 * file as the rule it bounds is a number they can weigh.
 */
const BASELINE = new Map<string, number>([
  ["frontend/src/app/errorboundary.tsx", 1],
  ["frontend/src/mcp-apps/story-hosts.tsx", 2],
  ["frontend/src/screens/addemploymentmodal.tsx", 1],
  ["frontend/src/screens/ai.tsx", 4],
  ["frontend/src/screens/aiexport.tsx", 1],
  ["frontend/src/screens/analytics.tsx", 4],
  ["frontend/src/screens/approvaleditor.tsx", 1],
  ["frontend/src/screens/archive.tsx", 2],
  ["frontend/src/screens/automationdetail.tsx", 1],
  ["frontend/src/screens/automations.datefield.tsx", 1],
  ["frontend/src/screens/billingcontactmodal.tsx", 1],
  ["frontend/src/screens/book.tsx", 24],
  ["frontend/src/screens/client.tsx", 6],
  ["frontend/src/screens/commissiondecide.tsx", 3],
  ["frontend/src/screens/common.tsx", 2],
  ["frontend/src/screens/companies.tsx", 5],
  ["frontend/src/screens/companydeepread.tsx", 13],
  ["frontend/src/screens/companyreject.tsx", 2],
  ["frontend/src/screens/contact360.tsx", 16],
  ["frontend/src/screens/contactcorrections.tsx", 20],
  ["frontend/src/screens/create.tsx", 2],
  ["frontend/src/screens/deal360/confirmadvance.tsx", 1],
  ["frontend/src/screens/deal360/dealactions.tsx", 4],
  ["frontend/src/screens/deal360/dealbrief.tsx", 2],
  ["frontend/src/screens/deal360/outcomereviewmodal.tsx", 1],
  ["frontend/src/screens/deals.tsx", 3],
  ["frontend/src/screens/edit.tsx", 1],
  ["frontend/src/screens/employmentedit.tsx", 1],
  ["frontend/src/screens/employmentimport.tsx", 1],
  ["frontend/src/screens/historyentries.tsx", 1],
  ["frontend/src/screens/historyfields.tsx", 11],
  ["frontend/src/screens/imap-connect-form.tsx", 1],
  ["frontend/src/screens/leads.list.tsx", 3],
  ["frontend/src/screens/leadsignals.tsx", 6],
  ["frontend/src/screens/listquery.tsx", 2],
  ["frontend/src/screens/merge.tsx", 5],
  ["frontend/src/screens/oauthconsent.tsx", 9],
  ["frontend/src/screens/offerlinebilling.tsx", 1],
  ["frontend/src/screens/offerlinerates.tsx", 2],
  ["frontend/src/screens/offers.tsx", 19],
  ["frontend/src/screens/onboarding-company-form.tsx", 5],
  ["frontend/src/screens/onboarding-conversation/connect-scene.tsx", 3],
  ["frontend/src/screens/partnercommissions.tsx", 3],
  ["frontend/src/screens/partners.tsx", 3],
  ["frontend/src/screens/projecthealthmodal.tsx", 1],
  ["frontend/src/screens/projectstakeholders.tsx", 3],
  ["frontend/src/screens/recordteamassign.tsx", 1],
  ["frontend/src/screens/relationshiprows.tsx", 7],
  ["frontend/src/screens/relationships.tsx", 7],
  ["frontend/src/screens/repeatablerowsfield.tsx", 4],
  ["frontend/src/screens/scheduledsends.tsx", 9],
  ["frontend/src/screens/share.tsx", 1],
  ["frontend/src/screens/strength.tsx", 14],
  ["frontend/src/screens/telegram-connect-form.tsx", 1],
  ["frontend/src/screens/transcriptread.tsx", 12],
]);

/**
 * What the whole tree carries, so a rise is ONE reviewable number rather than
 * fifty-odd entries a reader has to diff against each other.
 */
const TOTAL = 260;

/**
 * The two trees held at ZERO rather than baselined, by taking no entry at all:
 * an entry of nothing is what the census arm above already compares against, so
 * refusing the ENTRY is the whole rule. This directory publishes the
 * alternative, so a primitive laying itself out inline is the gate's own author
 * ignoring it; a unit ships no stylesheet, so `Stack` and `Row` are its only
 * spelling.
 */
const HELD_AT_ZERO = [/^frontend\/src\/design-system\//, /^extensions\//];

/** Files that are not the rendered product: their own suite and their own docs. */
const NOT_THE_PRODUCT = new RegExp(`\\.(test|stories)${MODULE_FILE.source}`);

/**
 * Every module the product renders: everything under `frontend/src`, plus each
 * unit's own frontend layer. Every dialect, not just `.tsx` — a unit may ship
 * `screen.jsx`, and a `.ts` module can hold a `CSSProperties` object.
 *
 * `.storybook/` stays out: it configures the catalog rather than shipping in the
 * app, and a decorator's frame is not a screen anybody reads.
 */
function modules(): string[] {
  const units = extensionLayers(extensionsRoot).flatMap((layer) =>
    filesUnder(layer),
  );
  return filesUnder(sourceRoot)
    .concat(units)
    .filter((path) => !NOT_THE_PRODUCT.test(path));
}

/** A path as the baseline spells it: relative to the repository, forward slashes. */
function pathOf(file: string): string {
  return relative(repoRoot, file).replaceAll("\\", "/");
}

/** `x as const`, `x satisfies T`, `(x)`, `x!` — wrappers that change no value. */
function unwrap(node: ts.Expression): ts.Expression {
  let current = node;
  while (
    ts.isAsExpression(current) ||
    ts.isSatisfiesExpression(current) ||
    ts.isParenthesizedExpression(current) ||
    ts.isTypeAssertionExpression(current) ||
    ts.isNonNullExpression(current)
  ) {
    current = current.expression;
  }
  return current;
}

/**
 * Whether this value is a rule rather than a reading. A no-substitution
 * template is the same literal the quotes would have made, and `-4` is a
 * literal the parser happens to hand over as an operator and a number.
 */
function isStatic(value: ts.Expression): boolean {
  const bare = unwrap(value);
  if (
    ts.isPrefixUnaryExpression(bare) &&
    (bare.operator === ts.SyntaxKind.MinusToken ||
      bare.operator === ts.SyntaxKind.PlusToken)
  ) {
    return ts.isNumericLiteral(bare.operand);
  }
  return (
    ts.isStringLiteral(bare) ||
    ts.isNumericLiteral(bare) ||
    ts.isNoSubstitutionTemplateLiteral(bare)
  );
}

/** The property this assignment names, including the computed constant form. */
function propertyName(name: ts.PropertyName): string | undefined {
  if (ts.isIdentifier(name) || ts.isStringLiteral(name)) return name.text;
  if (!ts.isComputedPropertyName(name)) return undefined;
  const key = unwrap(name.expression);
  return ts.isStringLiteral(key) || ts.isNoSubstitutionTemplateLiteral(key)
    ? key.text
    : undefined;
}

/**
 * Every value each name is given in one module. EVERY value, not the last: this
 * reader has no scope information, and dropping an ambiguous name would be the
 * census choosing the one direction it must not fail in.
 */
function initializers(source: ts.SourceFile): Map<string, ts.Expression[]> {
  const byName = new Map<string, ts.Expression[]>();
  const visit = (node: ts.Node) => {
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.initializer !== undefined
    ) {
      const known = byName.get(node.name.text);
      if (known) known.push(node.initializer);
      else byName.set(node.name.text, [node.initializer]);
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return byName;
}

/**
 * Every object literal an expression can hand to an element, however it is
 * wrapped: a ternary's two arms, a guard's right-hand side, a nullish default,
 * an `Object.assign`, a spread, a cast, and a name declared elsewhere in the
 * same module.
 *
 * It reaches THROUGH a call rather than stopping at one, so
 * `style={merge({ marginTop: "4px" })}` is read: the literal is a layout rule a
 * class could hold whatever the function does with it, and a walk that stopped
 * would hand every author a one-word way around this gate.
 */
function styleObjectsUnder(
  root: ts.Expression,
  named: Map<string, ts.Expression[]>,
): ts.ObjectLiteralExpression[] {
  const found: ts.ObjectLiteralExpression[] = [];
  const resolved = new Set<string>();
  const visit = (node: ts.Node): void => {
    // A property's NAME is an identifier too, so an object is walked by hand:
    // handing the whole node on would resolve `{ marginTop: x }` against a
    // module constant that happens to be called `marginTop`.
    if (ts.isObjectLiteralExpression(node)) {
      found.push(node);
      for (const property of node.properties) {
        if (ts.isPropertyAssignment(property)) visit(property.initializer);
        else if (ts.isSpreadAssignment(property)) visit(property.expression);
      }
      return;
    }
    if (ts.isPropertyAccessExpression(node)) {
      visit(node.expression);
      return;
    }
    if (ts.isIdentifier(node)) {
      if (resolved.has(node.text)) return;
      resolved.add(node.text);
      for (const initializer of named.get(node.text) ?? []) visit(initializer);
      return;
    }
    ts.forEachChild(node, visit);
  };
  visit(root);
  return found;
}

/** The call JSX compiles to, and the one a file may write by hand. */
const RENDER_CALL = /(^|\.)(createElement|jsx|jsxs|jsxDEV)$/;

/** The `style` a props object carries, however that object is written. */
function styleOfProps(
  props: ts.Expression,
  named: Map<string, ts.Expression[]>,
): ts.Expression[] {
  return styleObjectsUnder(props, named).flatMap((object) =>
    object.properties.flatMap((property) =>
      ts.isPropertyAssignment(property) &&
      propertyName(property.name) === "style"
        ? [property.initializer]
        : [],
    ),
  );
}

/**
 * Every expression handed to an element as its `style`, by each road the
 * language offers: the attribute, a spread of a props object, and the call
 * both of those compile to. An element cannot tell them apart and neither may
 * the census — a spread is otherwise one keystroke around this gate.
 */
function styleValues(
  node: ts.Node,
  named: Map<string, ts.Expression[]>,
): ts.Expression[] {
  if (ts.isJsxAttribute(node)) {
    if (!ts.isIdentifier(node.name) || node.name.text !== "style") return [];
    const value = node.initializer;
    if (value === undefined || !ts.isJsxExpression(value)) return [];
    return value.expression === undefined ? [] : [value.expression];
  }
  if (ts.isJsxSpreadAttribute(node))
    return styleOfProps(node.expression, named);
  if (
    ts.isCallExpression(node) &&
    RENDER_CALL.test(node.expression.getText())
  ) {
    const props = node.arguments[1];
    return props === undefined ? [] : styleOfProps(props, named);
  }
  return [];
}

/**
 * A style object written apart from any element: `const row: CSSProperties = …`.
 * Declared to BE a style, so it is one wherever it is used — including in a
 * `.ts` module with no JSX in it at all.
 */
function annotatedStyles(
  source: ts.SourceFile,
  named: Map<string, ts.Expression[]>,
): ts.ObjectLiteralExpression[] {
  const found: ts.ObjectLiteralExpression[] = [];
  const visit = (node: ts.Node) => {
    if (
      ts.isVariableDeclaration(node) &&
      node.type !== undefined &&
      node.initializer !== undefined &&
      STYLE_ANNOTATION.test(node.type.getText())
    ) {
      found.push(...styleObjectsUnder(node.initializer, named));
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

type InlineLayout = { where: string; line: number; property: string };

/** Every static layout rule written inline in one parsed module, by line. */
function inlineLayoutIn(where: string, source: ts.SourceFile): InlineLayout[] {
  const named = initializers(source);
  // A set, because one literal is reachable both from the element that wears it
  // and from its own annotated declaration, and it is one rule either way.
  const objects = new Set<ts.ObjectLiteralExpression>();
  const visit = (node: ts.Node) => {
    for (const style of styleValues(node, named)) {
      for (const object of styleObjectsUnder(style, named)) objects.add(object);
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  for (const object of annotatedStyles(source, named)) objects.add(object);

  const found: InlineLayout[] = [];
  for (const object of objects) {
    for (const property of object.properties) {
      if (!ts.isPropertyAssignment(property)) continue;
      const property_name = propertyName(property.name);
      if (property_name === undefined) continue;
      if (!LAYOUT_PROPERTY.test(property_name)) continue;
      if (!isStatic(property.initializer)) continue;
      found.push({
        where,
        line:
          source.getLineAndCharacterOfPosition(property.getStart()).line + 1,
        property: property_name,
      });
    }
  }
  return found.sort((a, b) => a.line - b.line);
}

function countsByFile(found: readonly InlineLayout[]): Map<string, number> {
  const counts = new Map<string, number>();
  for (const one of found) {
    counts.set(one.where, (counts.get(one.where) ?? 0) + 1);
  }
  return counts;
}

describe("layout is a class, not a style attribute", () => {
  const files = modules();
  const found = files.flatMap((file) =>
    inlineLayoutIn(pathOf(file), sourceFileAt(file)),
  );
  const counts = countsByFile(found);

  it("reads a corpus that is every rendered module in the tree", () => {
    // A census that read a smaller tree would report the same word, PASS. So
    // the floor sits just under the real count, and one file of each shape is
    // named: flat, nested, outside `screens/`, a `.ts` module with no JSX, and
    // a unit's own layer.
    const paths = files.map(pathOf);
    expect(files.length).toBeGreaterThan(900);
    expect(paths).toContain("frontend/src/screens/contact360.tsx");
    expect(paths).toContain("frontend/src/screens/deal360/dealactions.tsx");
    expect(paths).toContain("frontend/src/design-system/trust.tsx");
    expect(paths).toContain("frontend/src/design-system/anchoredpopup.ts");
    expect(paths).toContain("extensions/openchannel/frontend/screen.tsx");
  });

  it("carries the total it is pinned at", () => {
    expect(found.length).toBe(TOTAL);
  });

  it("carries no file over its baseline", () => {
    const over = [...counts]
      .filter(([where, count]) => count > (BASELINE.get(where) ?? 0))
      .flatMap(([where]) =>
        found
          .filter((one) => one.where === where)
          .map(
            (one) =>
              `${one.where}:${one.line}: \`${one.property}\` is laid out inline — ${FIX}`,
          ),
      );
    expect(over, over.join("\n")).toEqual([]);
  });

  it("keeps no baseline entry above what the tree carries", () => {
    const behind = [...BASELINE]
      .filter(([where, allowed]) => (counts.get(where) ?? 0) < allowed)
      .map(([where, allowed]) => {
        const count = counts.get(where) ?? 0;
        return count === 0
          ? `${where}: carries none — remove the entry`
          : `${where}: carries ${count}, not ${allowed} — lower the entry to ${count}`;
      });
    expect(behind, behind.join("\n")).toEqual([]);
  });

  it("baselines nothing in this directory or the unit tier", () => {
    const refused = [...BASELINE.keys()]
      .filter((where) => HELD_AT_ZERO.some((tier) => tier.test(where)))
      .map((where) => `${where}: this tier takes no baseline entry — ${FIX}`);
    expect(refused, refused.join("\n")).toEqual([]);
  });

  // The detector must be able to SEE each shape, or a clean tree and a blind
  // reader look identical.
  describe("the detector", () => {
    const read = (source: string): InlineLayout[] =>
      inlineLayoutIn("fixture.tsx", parseSource("fixture.tsx", source));

    it("reads a static rule, on one line and spread over many", () => {
      expect(
        read('<p style={{ marginTop: "var(--space-2)" }}>{note}</p>'),
      ).toEqual([{ where: "fixture.tsx", line: 1, property: "marginTop" }]);
      expect(
        read(
          '<div\n  style={{\n    display: "flex",\n    gap: "var(--space-2)",\n    alignItems: "center",\n  }}\n/>',
        ).map((one) => `${one.line}:${one.property}`),
      ).toEqual(["3:display", "4:gap", "5:alignItems"]);
    });

    it("reads a literal however the parser hands it over", () => {
      expect(read("<input style={{ width: 180 }} />")).toHaveLength(1);
      expect(read("<div style={{ padding: `0` }} />")).toHaveLength(1);
      expect(read("<div style={{ marginLeft: -4 }} />")).toHaveLength(1);
      expect(read('<div style={{ marginTop: ("4px") }} />')).toHaveLength(1);
      expect(
        read('<div style={{ marginTop: "4px" as const }} />'),
      ).toHaveLength(1);
      expect(read('<div style={{ ["marginTop"]: "4px" }} />')).toHaveLength(1);
      expect(read('<div style={{ "margin-top": "4px" }} />')).toHaveLength(1);
    });

    it("reads through every wrapper an element can hand its style", () => {
      const shapes = [
        '<div style={open ? { gap: "4px" } : undefined} />',
        '<div style={open && { gap: "4px" }} />',
        '<div style={given ?? { gap: "4px" }} />',
        '<div style={Object.assign({}, { gap: "4px" })} />',
        '<div style={{ gap: "4px" } as CSSProperties} />',
        '<div style={{ gap: "4px" } satisfies CSSProperties} />',
        '<div style={({ gap: "4px" })} />',
        '<div style={{ ...{ gap: "4px" } }} />',
        '<div style={{ ...(tight && { gap: "4px" }) }} />',
        '<div style={merge(base, { gap: "4px" })} />',
      ];
      expect(
        shapes.filter((shape) => read(shape).length !== 1),
        "a wrapper hid the rule",
      ).toEqual([]);
    });

    it("reads every road a style reaches an element by", () => {
      const roads = [
        '<div {...{ style: { marginTop: "4px" } }} />',
        'const props = { style: { gap: "8px" } };\n<div {...props} />',
        'createElement("div", { style: { padding: 0 } })',
        'const hoisted = { padding: 0 };\nReact.createElement("div", { style: hoisted })',
        'jsx("div", { style: { gap: "8px" } })',
      ];
      expect(
        roads.filter((road) => read(road).length !== 1),
        "a style reached its element unread",
      ).toEqual([]);
    });

    it("leaves a props object the render built alone", () => {
      expect(
        read("const frameProps = propsFor(frame);\n<div {...frameProps} />"),
      ).toEqual([]);
      expect(read("<div {...frameProps} />")).toEqual([]);
      expect(read('createElement("div", propsFor(frame))')).toEqual([]);
    });

    it("reads a style object hoisted to a name of its own", () => {
      expect(
        read('const row = { marginTop: "4px" };\n<div style={row} />'),
      ).toEqual([{ where: "fixture.tsx", line: 1, property: "marginTop" }]);
      expect(
        read(
          'const sheet = { row: { gap: "4px" } };\n<div style={sheet.row} />',
        ),
      ).toHaveLength(1);
    });

    it("reads a declared style with no element in sight", () => {
      expect(
        read('const row: CSSProperties = { marginTop: "4px" };'),
      ).toHaveLength(1);
      expect(
        read('const row: React.CSSProperties = { marginTop: "4px" };'),
      ).toHaveLength(1);
    });

    it("counts one rule once, however many ways it is reachable", () => {
      expect(
        read(
          'const row: CSSProperties = { marginTop: "4px" };\n<div style={row} />',
        ),
      ).toHaveLength(1);
    });

    it("leaves a value the render computed alone", () => {
      expect(read("<col style={{ width: widthOf(column) }} />")).toEqual([]);
      expect(read("<div style={{ width: frame.width }} />")).toEqual([]);
      // biome-ignore lint/suspicious/noTemplateCurlyInString: the substitution is the case under test — this string is source the walker parses, not a template
      expect(read("<div style={{ top: `${at.top}px` }} />")).toEqual([]);
      // biome-ignore lint/suspicious/noTemplateCurlyInString: as above — a bar filled to a percentage the render worked out
      expect(read("<span style={{ width: `${filled}%` }} />")).toEqual([]);
      expect(read("<div style={{ marginTop: tight ? 0 : 8 }} />")).toEqual([]);
      expect(read("<div style={{ width }} />")).toEqual([]);
      expect(read("<div style={{ ...frame }} />")).toEqual([]);
      expect(read("<div style={frameStyle} />")).toEqual([]);
    });

    it("leaves a property that lays nothing out alone", () => {
      expect(read('<span style={{ userSelect: "none" }} />')).toEqual([]);
      expect(read('<p style={{ color: "var(--dangerText)" }} />')).toEqual([]);
      expect(
        read('<text style={{ fontWeight: "var(--fontWeightBold)" }} />'),
      ).toEqual([]);
      expect(read('<div style={{ opacity: "0" }} />')).toEqual([]);
    });

    it("reads the whole family, not the properties somebody listed", () => {
      const families = [
        '<div style={{ marginInlineStart: "0" }} />',
        '<div style={{ gridColumn: "1" }} />',
        '<div style={{ position: "relative" }} />',
        '<div style={{ overflowX: "hidden" }} />',
        '<div style={{ float: "left" }} />',
        "<div style={{ order: 2 }} />",
        '<div style={{ boxSizing: "border-box" }} />',
        '<div style={{ aspectRatio: "16 / 9" }} />',
        '<div style={{ transform: "translateY(-2px)" }} />',
        '<div style={{ translate: "0 -2px" }} />',
        "<div style={{ zIndex: 2 }} />",
        '<div style={{ inlineSize: "8rem" }} />',
        '<div style={{ blockSize: "8rem" }} />',
        '<div style={{ minInlineSize: "8rem" }} />',
        '<div style={{ maxInlineSize: "8rem" }} />',
        '<div style={{ minBlockSize: "8rem" }} />',
        '<div style={{ maxBlockSize: "8rem" }} />',
        '<div style={{ marginInline: "auto" }} />',
        '<div style={{ paddingBlock: "4px" }} />',
        "<div style={{ insetInlineStart: 0 }} />",
      ];
      expect(
        families.filter((one) => read(one).length !== 1),
        "a layout family went unread",
      ).toEqual([]);
    });

    it("reads a unit's own dialect", () => {
      const jsx = inlineLayoutIn(
        "screen.jsx",
        parseSource("screen.jsx", '<div style={{ gap: "4px" }} />'),
      );
      expect(jsx).toHaveLength(1);
    });
  });
});
