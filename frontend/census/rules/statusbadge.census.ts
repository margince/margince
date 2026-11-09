// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import ts from "typescript";
import { moduleSpecifiers } from "../../scripts/lib/source-tree";
import type { CensusRule } from "../tree";

// Blue never means status. A module that uses this palette may not keep a
// second map beside it that still hands out a blue tone.
const BLUE = new Set(["info", "accent"]);
const STATUS_TONES = new Set(["success", "warning", "danger", "default"]);

function literalOf(node: ts.Node | undefined): string | null {
  return node && ts.isStringLiteralLike(node) ? node.text : null;
}

function namedTone(name: ts.Node): boolean {
  return (
    (ts.isIdentifier(name) || ts.isStringLiteral(name)) && name.text === "tone"
  );
}

/** A tone union or tone map typed out by hand, with blue among the statuses. */
function blueAmongStatuses(values: (string | null)[]): boolean {
  const known = values.filter((value): value is string => value !== null);
  return (
    known.some((value) => BLUE.has(value)) &&
    known.some((value) => STATUS_TONES.has(value))
  );
}

function blueToneAt(node: ts.Node): boolean {
  // A Callout or Panel may still be blue; only a Badge's tone is a status.
  if (ts.isJsxAttribute(node) && namedTone(node.name)) {
    const element = node.parent.parent;
    return (
      element.tagName.getText(node.getSourceFile()) === "Badge" &&
      BLUE.has(literalOf(node.initializer) ?? "")
    );
  }
  if (ts.isPropertyAssignment(node) && namedTone(node.name)) {
    return BLUE.has(literalOf(node.initializer) ?? "");
  }
  if (ts.isUnionTypeNode(node)) {
    return blueAmongStatuses(
      node.types.map((member) =>
        ts.isLiteralTypeNode(member) ? literalOf(member.literal) : null,
      ),
    );
  }
  if (ts.isObjectLiteralExpression(node)) {
    return blueAmongStatuses(
      node.properties.map((property) =>
        ts.isPropertyAssignment(property)
          ? literalOf(property.initializer)
          : null,
      ),
    );
  }
  return false;
}

export function blueStatusLines(source: ts.SourceFile): number[] {
  const lines: number[] = [];
  const visit = (node: ts.Node): void => {
    if (blueToneAt(node)) {
      lines.push(
        source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1,
      );
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return lines;
}

function usesPalette(source: ts.SourceFile): boolean {
  return moduleSpecifiers(source, "all").some((specifier) =>
    /(^|\/)statusbadge$/.test(specifier),
  );
}

export const rule: CensusRule = {
  name: "statusbadge",
  corpus: (tree) =>
    tree.files.filter(
      (file) => file.area === "src" && file.source !== undefined,
    ),
  floors: (corpus) => [
    ...(corpus.length > 500 ? [] : ["fewer than 500 modules were read"]),
    ...(corpus.some(
      (file) =>
        file.rel === "src/design-system/statusbadge.stories.tsx" &&
        file.source !== undefined &&
        usesPalette(file.source),
    )
      ? []
      : ["statusbadge.stories.tsx was not read as a user of the palette"]),
  ],
  visit: (file) =>
    file.source && usesPalette(file.source)
      ? blueStatusLines(file.source).map((line) => `${file.rel}:${line}`)
      : [],
  advice:
    "Blue never means status. Give a status the tone StatusBadge gives it " +
    "(statusTone), and keep no second map that hands out info or accent.",
};
