// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join } from "node:path";
import ts from "typescript";
import { en } from "../../src/i18n/en.ts";
import { LOCALES } from "../../src/i18n/locale.ts";
import { initialEntries } from "../vite-entry-chunks.ts";
import { aliases, moduleGraph } from "./bundle-reach.ts";
import { filesUnder, parseSource } from "./source-tree.ts";

export function essentialKeys(root: string): ReadonlySet<string> {
  const src = join(root, "src");
  const tree = new Map(
    filesUnder(src).map((file) => [file, readFileSync(file, "utf8")]),
  );
  const entries = Object.values(initialEntries(root)).flat();
  for (const entry of entries) {
    if (!tree.has(entry)) throw new Error(`Missing initial entry: ${entry}`);
  }
  const graph = moduleGraph({
    tree,
    aliases: aliases(),
    catalogs: new Set(
      LOCALES.map((locale) => join(src, "i18n", `${locale}.ts`)),
    ),
    dynamicImports: false,
  });
  const modules = graph.reach(entries);
  for (const path of modules.keys()) {
    if (
      path === join(src, "App.screens.tsx") ||
      path === join(src, "i18n/lazy.tsx")
    )
      continue;
    const source = tree.get(path);
    if (source === undefined)
      throw new Error(`Missing reached module: ${path}`);
    requireGuardedLazy(path, source);
  }

  const literals = new Set(
    [...modules.values()].flatMap((module) => module.literals),
  );
  const stems = new Set(
    [...modules.values()].flatMap((module) => module.stems),
  );
  for (const literal of literals) {
    if (literal.endsWith(".")) stems.add(literal);
  }
  const keys = Object.keys(en).filter(
    (key) =>
      literals.has(key) ||
      literals.has(key.replace(/_(one|other)$/, "")) ||
      [...stems].some((stem) => key.startsWith(stem)),
  );
  if (keys.length === 0)
    throw new Error("Initial entries requested no catalog messages");
  return new Set(keys);
}

function requireGuardedLazy(path: string, source: string): void {
  const parsed = parseSource(path, source);
  const namespaces = new Set<string>();
  for (const node of parsed.statements) {
    if (
      !ts.isImportDeclaration(node) ||
      !ts.isStringLiteral(node.moduleSpecifier) ||
      node.moduleSpecifier.text !== "react"
    )
      continue;
    const clause = node.importClause;
    if (clause?.name) namespaces.add(clause.name.text);
    const bindings = clause?.namedBindings;
    if (bindings && ts.isNamespaceImport(bindings))
      namespaces.add(bindings.name.text);
    if (
      bindings &&
      ts.isNamedImports(bindings) &&
      bindings.elements.some(
        (item) => (item.propertyName ?? item.name).text === "lazy",
      )
    )
      throw new Error(`Initial lazy components must use i18n/lazy: ${path}`);
  }
  if (usesNamespaceLazy(parsed, namespaces))
    throw new Error(`Initial lazy components must use i18n/lazy: ${path}`);
}

function usesNamespaceLazy(
  source: ts.Node,
  names: ReadonlySet<string>,
): boolean {
  if (
    ts.isPropertyAccessExpression(source) &&
    ts.isIdentifier(source.expression) &&
    names.has(source.expression.text) &&
    source.name.text === "lazy"
  )
    return true;
  if (
    ts.isElementAccessExpression(source) &&
    ts.isIdentifier(source.expression) &&
    names.has(source.expression.text) &&
    ts.isStringLiteral(source.argumentExpression) &&
    source.argumentExpression.text === "lazy"
  )
    return true;
  return (
    ts.forEachChild(
      source,
      (child) => usesNamespaceLazy(child, names) || undefined,
    ) ?? false
  );
}
