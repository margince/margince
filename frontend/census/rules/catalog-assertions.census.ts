// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import ts from "typescript";
import type { CensusRule } from "../tree";
import { srcDir } from "../tree";

export const MIN_CATALOG_ASSERTION_LENGTH = 12;

export function catalogAssertionValues(
  source: ts.SourceFile,
): Map<string, string[]> {
  const values = new Map<string, string[]>();
  const visit = (node: ts.Node) => {
    if (
      ts.isPropertyAssignment(node) &&
      ts.isStringLiteral(node.name) &&
      (ts.isStringLiteral(node.initializer) ||
        ts.isNoSubstitutionTemplateLiteral(node.initializer))
    ) {
      const text = node.initializer.text;
      if (text.length >= MIN_CATALOG_ASSERTION_LENGTH) {
        values.set(text, [...(values.get(text) ?? []), node.name.text]);
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return values;
}

function isAssertionText(node: ts.Node): boolean {
  for (let parent = node.parent; parent; parent = parent.parent) {
    if (ts.isCallExpression(parent)) {
      const call = parent.expression;
      const name = ts.isPropertyAccessExpression(call)
        ? call.name.text
        : ts.isIdentifier(call)
          ? call.text
          : "";
      if (
        /^(get|find|query)(All)?By/.test(name) ||
        /^to(HaveTextContent|Contain|Equal|Be|Match)/.test(name)
      ) {
        return true;
      }
    }
    if (
      ts.isStatement(parent) &&
      !ts.isExpressionStatement(parent) &&
      !ts.isVariableStatement(parent)
    ) {
      break;
    }
  }
  return false;
}

function fixtureTextValues(source: ts.SourceFile): ReadonlySet<string> {
  const found = new Set<string>();
  const fixtureNames = new Set([
    "name",
    "label",
    "title",
    "description",
    "full_name",
    "display_name",
    "subject",
    "body",
    "children",
  ]);
  const visit = (node: ts.Node) => {
    if (
      (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) &&
      !isAssertionText(node)
    ) {
      const parent = node.parent;
      if (ts.isJsxAttribute(parent) && parent.initializer === node) {
        found.add(node.text);
      }
      if (
        ts.isPropertyAssignment(parent) &&
        parent.initializer === node &&
        (ts.isIdentifier(parent.name) || ts.isStringLiteral(parent.name)) &&
        fixtureNames.has(parent.name.text)
      ) {
        found.add(node.text);
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

export function literalCatalogAssertions(
  source: ts.SourceFile,
  values: ReadonlyMap<string, readonly string[]>,
): Array<{
  node: ts.StringLiteral | ts.NoSubstitutionTemplateLiteral;
  keys: readonly string[];
}> {
  const fixtures = fixtureTextValues(source);
  const found: Array<{
    node: ts.StringLiteral | ts.NoSubstitutionTemplateLiteral;
    keys: readonly string[];
  }> = [];
  const visit = (node: ts.Node) => {
    if (
      (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) &&
      !(ts.isPropertyAssignment(node.parent) && node.parent.name === node) &&
      !ts.isTaggedTemplateExpression(node.parent) &&
      isAssertionText(node)
    ) {
      const keys = values.get(node.text);
      if (keys && !fixtures.has(node.text)) {
        found.push({ node, keys });
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

export const rule: CensusRule = {
  name: "catalog-assertions",
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        file.source !== undefined && /\.(test|stories)\.tsx$/.test(file.rel),
    ),
  floors: (corpus, tree) => [
    ...(corpus.length > 500
      ? []
      : ["fewer than 500 test and story modules were read"]),
    ...(tree.at(join(srcDir, "i18n/en.ts"))?.source
      ? []
      : ["the English catalog was not read"]),
  ],
  across: (corpus, tree) => {
    const catalog = tree.at(join(srcDir, "i18n/en.ts"))?.source;
    if (!catalog) {
      return ["the English catalog could not be parsed"];
    }
    const values = catalogAssertionValues(catalog);
    if (values.size < 1000) {
      return [`only ${values.size} long catalog values were read`];
    }
    return corpus.flatMap((file) => {
      const source = file.source;
      if (!source) {
        return [];
      }
      return literalCatalogAssertions(source, values).map(
        ({ node, keys }) =>
          `${file.rel}:${source.getLineAndCharacterOfPosition(node.getStart()).line + 1}: catalog text is repeated as a literal; read ${keys.join(" or ")}`,
      );
    });
  },
  advice:
    "Read rendered English and absence checks through en[key] or translate(). Fixture data outside assertions may stay literal.",
};
