// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  extensionLayers,
  filesMatching,
  parseSource,
  sourceFileAt,
} from "../../scripts/lib/source-tree";

// LAYOUT IS A CLASS, NOT A STYLE ATTRIBUTE, AND THE COUNT ONLY FALLS.
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
 * how it flows, and where it sits. Matched as a PREFIX, so `marginInlineStart`
 * and `gridTemplateColumns` are covered without a list that goes stale the
 * day the platform ships another one.
 */
const LAYOUT_PROPERTY =
  /^(margin|padding|gap|rowGap|columnGap|display|flex|grid|justify|align|place|inset|top|right|bottom|left|width|minWidth|maxWidth|height|minHeight|maxHeight)/;

/** What the fix is, said the same way at every finding. */
const FIX =
  "a class in the screen's sheet, or `Stack`/`Row` from design-system/stack.tsx";

/**
 * How many inline layout rules each file still carries. The number may FALL and
 * never rise: a file over its entry fails, and so does one under it, because a
 * baseline left high is a budget the next author spends without deciding to.
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
  ["frontend/src/screens/approvaleditor.tsx", 3],
  ["frontend/src/screens/approvalrow.tsx", 5],
  ["frontend/src/screens/archive.tsx", 2],
  ["frontend/src/screens/automationdetail.tsx", 1],
  ["frontend/src/screens/automations.datefield.tsx", 1],
  ["frontend/src/screens/billingcontactmodal.tsx", 1],
  ["frontend/src/screens/book.tsx", 24],
  ["frontend/src/screens/client.tsx", 7],
  ["frontend/src/screens/commissiondecide.tsx", 3],
  ["frontend/src/screens/common.tsx", 2],
  ["frontend/src/screens/companies.tsx", 5],
  ["frontend/src/screens/companydeepread.tsx", 13],
  ["frontend/src/screens/companyreject.tsx", 2],
  ["frontend/src/screens/contact360.tsx", 16],
  ["frontend/src/screens/contactcorrections.tsx", 21],
  ["frontend/src/screens/create.tsx", 2],
  ["frontend/src/screens/deal360/confirmadvance.tsx", 1],
  ["frontend/src/screens/deal360/dealactions.tsx", 4],
  ["frontend/src/screens/deal360/outcomereviewmodal.tsx", 1],
  ["frontend/src/screens/deals.tsx", 5],
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
  ["frontend/src/screens/offers.tsx", 22],
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
 * The two trees that are held at ZERO rather than baselined. This directory
 * publishes the alternative, so a primitive writing layout inline is the gate's
 * own author ignoring it; a unit has no stylesheet at all, so `Stack` and `Row`
 * are its only spelling and an inline rule there is a step nobody can restyle.
 */
const HELD_AT_ZERO = [/^frontend\/src\/design-system\//, /^extensions\//];

/** Every rendered module: the whole frontend tree and every unit's layer. */
function modules(): string[] {
  const units = extensionLayers(extensionsRoot).flatMap((layer) =>
    filesMatching(layer, /\.tsx$/),
  );
  return filesMatching(sourceRoot, /\.tsx$/)
    .concat(units)
    .filter((path) => !/\.(test|stories)\.tsx$/.test(path));
}

/** A path as the baseline spells it: relative to the repository, forward slashes. */
function pathOf(file: string): string {
  return relative(repoRoot, file).replaceAll("\\", "/");
}

/**
 * Whether this value is a rule rather than a reading. A no-substitution
 * template is the same literal the quotes would have made, so it counts; one
 * with a `${}` in it is arithmetic the render did.
 */
function isStatic(value: ts.Expression): boolean {
  return (
    ts.isStringLiteral(value) ||
    ts.isNumericLiteral(value) ||
    ts.isNoSubstitutionTemplateLiteral(value)
  );
}

/** The object literal a `style` attribute holds, if it holds one at all. */
function styleObject(node: ts.Node): ts.ObjectLiteralExpression | undefined {
  if (!ts.isJsxAttribute(node) || !ts.isIdentifier(node.name)) return undefined;
  if (node.name.text !== "style") return undefined;
  const value = node.initializer;
  if (value === undefined || !ts.isJsxExpression(value)) return undefined;
  const expression = value.expression;
  return expression !== undefined && ts.isObjectLiteralExpression(expression)
    ? expression
    : undefined;
}

type InlineLayout = { where: string; line: number; property: string };

/** Every static layout rule written inline in one parsed module, in source order. */
function inlineLayoutIn(where: string, source: ts.SourceFile): InlineLayout[] {
  const found: InlineLayout[] = [];
  const visit = (node: ts.Node) => {
    const object = styleObject(node);
    for (const property of object?.properties ?? []) {
      // A spread and a shorthand both carry a name the render computed, so
      // neither is a rule this gate can ask to move.
      if (!ts.isPropertyAssignment(property)) continue;
      const name =
        ts.isIdentifier(property.name) || ts.isStringLiteral(property.name)
          ? property.name.text
          : undefined;
      if (name === undefined || !LAYOUT_PROPERTY.test(name)) continue;
      if (!isStatic(property.initializer)) continue;
      found.push({
        where,
        line:
          source.getLineAndCharacterOfPosition(property.getStart()).line + 1,
        property: name,
      });
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
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
    // the floor is named, and so is one file from each shape of the corpus:
    // flat, nested, outside `screens/`, and in a unit's own layer.
    const paths = files.map(pathOf);
    expect(files.length).toBeGreaterThan(500);
    expect(paths).toContain("frontend/src/screens/contact360.tsx");
    expect(paths).toContain("frontend/src/screens/deal360/dealactions.tsx");
    expect(paths).toContain("frontend/src/design-system/trust.tsx");
    expect(paths).toContain("extensions/openchannel/frontend/screen.tsx");
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

  it("holds this directory and the unit tier at zero", () => {
    const heldAtZero = (where: string) =>
      HELD_AT_ZERO.some((tier) => tier.test(where));
    const findings = found
      .filter((one) => heldAtZero(one.where))
      .map(
        (one) =>
          `${one.where}:${one.line}: \`${one.property}\` — this tier is not baselined: ${FIX}`,
      );
    const baselined = [...BASELINE.keys()]
      .filter(heldAtZero)
      .map((where) => `${where}: this tier takes no baseline entry`);
    const refused = [...findings, ...baselined];
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

    it("reads a number and a template with nothing in it", () => {
      expect(read("<input style={{ width: 180 }} />")).toHaveLength(1);
      expect(read("<div style={{ padding: `0` }} />")).toHaveLength(1);
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
    });

    it("leaves a property that lays nothing out alone", () => {
      expect(read('<span style={{ userSelect: "none" }} />')).toEqual([]);
      expect(read('<p style={{ color: "var(--dangerText)" }} />')).toEqual([]);
      expect(
        read('<text style={{ fontWeight: "var(--fontWeightBold)" }} />'),
      ).toEqual([]);
    });

    it("reads the whole family, not the properties somebody listed", () => {
      expect(
        read('<div style={{ marginInlineStart: "0", gridColumn: "1" }} />'),
      ).toHaveLength(2);
    });

    it("leaves a style that is not an object literal alone", () => {
      expect(read("<div style={frameStyle} />")).toEqual([]);
    });
  });
});
