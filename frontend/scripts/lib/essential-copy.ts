// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join } from "node:path";
import ts from "typescript";
import { en } from "../../src/i18n/en.ts";
import { LOCALES } from "../../src/i18n/locale.ts";
import { initialEntries } from "../vite-entry-chunks.ts";
import { aliases, moduleGraph } from "./bundle-reach.ts";
import {
  filesUnder,
  moduleSpecifiers,
  parseSource,
  resolveRelative,
} from "./source-tree.ts";

export function essentialKeys(root: string): ReadonlySet<string> {
  const src = join(root, "src");
  const tree = new Map(
    filesUnder(src).map((file) => [file, readFileSync(file, "utf8")]),
  );
  const modules = initialModules(root, tree);

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

function requireGuardedLazy(path: string, parsed: ts.SourceFile): void {
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
  if (namespaceLazy(source, names)) return true;
  return (
    ts.forEachChild(
      source,
      (child) => usesNamespaceLazy(child, names) || undefined,
    ) ?? false
  );
}

function essentialLazyImports(
  path: string,
  source: ts.SourceFile,
  boundary: string,
  tree: ReadonlyMap<string, string>,
): string[] {
  const { names, namespaces } = lazyBindings(path, source, boundary, tree);
  const roots: string[] = [];
  const walk = (node: ts.Node): void => {
    const factory = essentialFactory(node, names, namespaces);
    if (factory !== undefined) {
      const imports = moduleSpecifiers(factory, "values");
      if (imports.length === 0)
        throw new Error(
          `Essential lazy component must name its imported module: ${path}`,
        );
      for (const specifier of imports) {
        const target = resolveRelative(path, specifier, (file) =>
          tree.has(file),
        );
        if (target === null)
          throw new Error(
            `Essential lazy import cannot be resolved: ${path}: ${specifier}`,
          );
        roots.push(target);
      }
    }
    ts.forEachChild(node, walk);
  };
  walk(source);
  return roots;
}

function namespaceLazy(node: ts.Node, names: ReadonlySet<string>): boolean {
  return (
    (ts.isPropertyAccessExpression(node) &&
      ts.isIdentifier(node.expression) &&
      names.has(node.expression.text) &&
      node.name.text === "lazy") ||
    (ts.isElementAccessExpression(node) &&
      ts.isIdentifier(node.expression) &&
      names.has(node.expression.text) &&
      ts.isStringLiteralLike(node.argumentExpression) &&
      node.argumentExpression.text === "lazy")
  );
}

function initialModules(root: string, tree: ReadonlyMap<string, string>) {
  const src = join(root, "src");
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
  const modules = new Map(graph.reach(entries));
  const pending = [...modules.keys()];
  for (let path = pending.pop(); path !== undefined; path = pending.pop()) {
    if (
      path === join(src, "App.screens.tsx") ||
      path === join(src, "i18n/lazy.tsx")
    )
      continue;
    const source = tree.get(path);
    if (source === undefined)
      throw new Error(`Missing reached module: ${path}`);
    const parsed = parseSource(path, source);
    requireGuardedLazy(path, parsed);
    const deferred = essentialLazyImports(
      path,
      parsed,
      join(src, "i18n/lazy.tsx"),
      tree,
    );
    for (const [target, module] of graph.reach(deferred)) {
      if (modules.has(target)) continue;
      modules.set(target, module);
      pending.push(target);
    }
  }

  return modules;
}

function lazyBindings(
  path: string,
  source: ts.SourceFile,
  boundary: string,
  tree: ReadonlyMap<string, string>,
) {
  const names = new Set<string>();
  const namespaces = new Set<string>();
  for (const node of source.statements) {
    const bindings = importedBindings(path, node, boundary, tree);
    if (bindings === undefined) continue;
    if (ts.isNamespaceImport(bindings)) {
      namespaces.add(bindings.name.text);
      continue;
    }
    for (const item of bindings.elements) {
      if ((item.propertyName ?? item.name).text === "lazy")
        names.add(item.name.text);
    }
  }

  return { names, namespaces };
}

function essentialFactory(
  node: ts.Node,
  names: ReadonlySet<string>,
  namespaces: ReadonlySet<string>,
): ts.Expression | undefined {
  if (!ts.isCallExpression(node)) return undefined;
  const named =
    ts.isIdentifier(node.expression) && names.has(node.expression.text);
  if (!named && !namespaceLazy(node.expression, namespaces)) return undefined;
  const level = node.arguments[1];
  if (
    level === undefined ||
    (ts.isStringLiteralLike(level) && level.text !== "essential")
  )
    return undefined;
  return node.arguments[0];
}

function importedBindings(
  path: string,
  node: ts.Statement,
  boundary: string,
  tree: ReadonlyMap<string, string>,
): ts.NamedImportBindings | undefined {
  if (
    !ts.isImportDeclaration(node) ||
    !ts.isStringLiteral(node.moduleSpecifier)
  )
    return undefined;
  if (
    resolveRelative(path, node.moduleSpecifier.text, (file) =>
      tree.has(file),
    ) !== boundary
  )
    return undefined;
  return node.importClause?.namedBindings;
}
