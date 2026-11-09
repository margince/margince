// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A list of caches to invalidate is easy to forget to add to. A new
// model-written surface missing from it keeps its old language after the
// setting changes. So the candidates come from the tree: every query key whose
// name reads like a model-written surface must be in the list.

import ts from "typescript";
import { resolveRelative } from "../../scripts/lib/source-tree";
import { LANGUAGE_DEPENDENT_QUERY_PREFIXES } from "../../src/features/sign-in/aicaches";
import { SCREEN_ROOTS } from "../../src/testing/screenfiles";
import type { CensusFile, CensusRule, CensusTree } from "../tree";

// The screen folders, and lib/ where shared queries live, at any depth.
const QUERY_ROOTS = [...SCREEN_ROOTS, "lib"].map((root) => `src/${root}/`);

// Matched against the first segment of a query key, which names the resource.
const MODEL_WRITTEN = /^(.*brief|.*dossier|.*growth-fit|deal-status)$/i;

// Query keys that match the pattern and are not model-written prose, each with
// the reason, so a finding cannot be silenced by a bare line.
const NOT_PROSE: Record<string, string> = {};

type SourceReader = (path: string) => ts.SourceFile | undefined;
type ModuleResolver = (from: string, specifier: string) => string | null;

export function queryPrefixes(
  source: ts.SourceFile,
  read: SourceReader,
  resolve: ModuleResolver,
): string[] {
  const found: string[] = [];
  const visit = (node: ts.Node) => {
    if (
      ts.isPropertyAssignment(node) &&
      node.name.getText(source) === "queryKey"
    ) {
      const key = queryPrefix(node.initializer, read, resolve, new Set());
      if (key !== null) found.push(key);
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

function queryPrefix(
  expression: ts.Expression,
  read: SourceReader,
  resolve: ModuleResolver,
  seen: Set<ts.Node>,
): string | null {
  if (seen.has(expression)) return null;
  seen.add(expression);
  if (
    ts.isAsExpression(expression) ||
    ts.isSatisfiesExpression(expression) ||
    ts.isParenthesizedExpression(expression)
  ) {
    return queryPrefix(expression.expression, read, resolve, seen);
  }
  if (ts.isArrayLiteralExpression(expression)) {
    const first = expression.elements[0];
    if (!first) return null;
    if (ts.isStringLiteralLike(first)) return first.text;
    if (ts.isSpreadElement(first))
      return queryPrefix(first.expression, read, resolve, seen);
    return null;
  }
  if (!ts.isIdentifier(expression)) return null;
  const bound = queryKeyBinding(expression.text, expression, read, resolve);
  return bound ? queryPrefix(bound, read, resolve, seen) : null;
}

function queryKeyBinding(
  name: string,
  at: ts.Node,
  read: SourceReader,
  resolve: ModuleResolver,
): ts.Expression | null {
  for (
    let scope: ts.Node | undefined = at.parent;
    scope;
    scope = scope.parent
  ) {
    if (
      ts.isFunctionLike(scope) &&
      scope.parameters.some((parameter) => bindsName(parameter.name, name))
    )
      return null;
    if (
      ts.isCatchClause(scope) &&
      scope.variableDeclaration &&
      bindsName(scope.variableDeclaration.name, name)
    )
      return null;
    if (
      ts.isForStatement(scope) ||
      ts.isForOfStatement(scope) ||
      ts.isForInStatement(scope)
    ) {
      const declarations = scope.initializer;
      if (
        declarations &&
        ts.isVariableDeclarationList(declarations) &&
        declarations.declarations.some((entry) => bindsName(entry.name, name))
      )
        return null;
    }
    if (!ts.isBlock(scope) && !ts.isSourceFile(scope)) continue;
    for (const statement of scope.statements) {
      if (!ts.isVariableStatement(statement)) continue;
      const declaration = statement.declarationList.declarations.find((entry) =>
        bindsName(entry.name, name),
      );
      if (declaration)
        return ts.isIdentifier(declaration.name) &&
          statement.declarationList.flags & ts.NodeFlags.Const
          ? (declaration.initializer ?? null)
          : null;
    }
    if (ts.isSourceFile(scope)) return importedKey(name, scope, read, resolve);
  }
  return null;
}

function bindsName(binding: ts.BindingName, name: string): boolean {
  if (ts.isIdentifier(binding)) return binding.text === name;
  return binding.elements.some(
    (element) =>
      !ts.isOmittedExpression(element) && bindsName(element.name, name),
  );
}

function exportedKey(
  source: ts.SourceFile,
  name: string,
): ts.Expression | null {
  for (const statement of source.statements) {
    if (
      ts.isVariableStatement(statement) &&
      statement.modifiers?.some(
        (modifier) => modifier.kind === ts.SyntaxKind.ExportKeyword,
      )
    ) {
      const value = constantNamed(statement, name);
      if (value) return value;
    }
    if (
      !ts.isExportDeclaration(statement) ||
      statement.isTypeOnly ||
      statement.moduleSpecifier ||
      !statement.exportClause ||
      !ts.isNamedExports(statement.exportClause)
    )
      continue;
    const exported = statement.exportClause.elements.find(
      (entry) => !entry.isTypeOnly && entry.name.text === name,
    );
    if (!exported) continue;
    const local = exported.propertyName?.text ?? exported.name.text;
    for (const declaration of source.statements) {
      if (!ts.isVariableStatement(declaration)) continue;
      const value = constantNamed(declaration, local);
      if (value) return value;
    }
  }
  return null;
}

function constantNamed(
  statement: ts.VariableStatement,
  name: string,
): ts.Expression | null {
  if (!(statement.declarationList.flags & ts.NodeFlags.Const)) return null;
  const declaration = statement.declarationList.declarations.find(
    (entry) => ts.isIdentifier(entry.name) && entry.name.text === name,
  );
  return declaration?.initializer ?? null;
}

function importedKey(
  name: string,
  source: ts.SourceFile,
  read: SourceReader,
  resolve: ModuleResolver,
): ts.Expression | null {
  for (const statement of source.statements) {
    if (
      !ts.isImportDeclaration(statement) ||
      !ts.isStringLiteralLike(statement.moduleSpecifier)
    )
      continue;
    const bindings = statement.importClause?.namedBindings;
    if (!bindings || !ts.isNamedImports(bindings)) continue;
    const binding = bindings.elements.find((entry) => entry.name.text === name);
    if (!binding) continue;
    const path = resolve(source.fileName, statement.moduleSpecifier.text);
    if (!path) return null;
    const owner = read(path);
    if (!owner) return null;
    const exported = binding.propertyName?.text ?? binding.name.text;
    return exportedKey(owner, exported);
  }
  return null;
}

const inCorpus = (file: CensusFile) =>
  file.area === "src" &&
  QUERY_ROOTS.some((root) => file.rel.startsWith(root)) &&
  /\.tsx?$/.test(file.path) &&
  !/\.test\.tsx?$/.test(file.path);

/** The tree's own reader and resolver, so an imported key is read from the parse. */
export function treeReaders(tree: CensusTree): [SourceReader, ModuleResolver] {
  return [
    (path) => tree.at(path)?.source,
    (from, specifier) =>
      resolveRelative(from, specifier, (path) => tree.at(path) !== undefined),
  ];
}

export function findings(
  queried: readonly { key: string; file: string }[],
): string[] {
  const named = new Set<string>(
    LANGUAGE_DEPENDENT_QUERY_PREFIXES.map((prefix) => prefix[0]),
  );
  const missing = queried
    .filter(
      ({ key }) =>
        MODEL_WRITTEN.test(key) && !named.has(key) && !(key in NOT_PROSE),
    )
    .map(
      ({ key, file }) =>
        `${file}: ${key} looks model-written and is not in LANGUAGE_DEPENDENT_QUERY_PREFIXES`,
    );
  // A prefix nobody queries invalidates nothing and reads as coverage.
  const keys = new Set(queried.map(({ key }) => key));
  const stale = [...named]
    .filter((key) => !keys.has(key))
    .map((key) => `${key} is invalidated but no screen queries it`);
  return [...missing, ...stale];
}

export const rule: CensusRule = {
  name: "aicaches",
  corpus: (tree) => tree.files.filter(inCorpus),
  across: (corpus, tree) => {
    const [read, resolve] = treeReaders(tree);
    return findings(
      corpus.flatMap((file) =>
        file.source
          ? queryPrefixes(file.source, read, resolve).map((key) => ({
              key,
              file: file.rel,
            }))
          : [],
      ),
    );
  },
  advice:
    "Changing the installation's base language invalidates the caches in " +
    "LANGUAGE_DEPENDENT_QUERY_PREFIXES (src/features/sign-in/aicaches.ts). " +
    "Add a model-written surface to that list, or add it to NOT_PROSE with " +
    "what it returns; drop a prefix no screen queries any more.",
};
