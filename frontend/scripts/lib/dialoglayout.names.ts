// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The names one file declares and the names it takes from elsewhere. Both are
// read per file and cached, because a dialog walk asks for them once per tag.

import ts from "typescript";

// Every name a module imports or re-exports, with where it comes from. An
// `export { A as B }` with no `from` points back into the module itself.
export function namedIn(s: ts.Statement) {
  if (!ts.isImportDeclaration(s) && !ts.isExportDeclaration(s)) return [];
  const spec = s.moduleSpecifier;
  const named = ts.isImportDeclaration(s)
    ? s.importClause?.namedBindings
    : s.exportClause;
  if ((spec && !ts.isStringLiteral(spec)) || !named) return [];
  if (!ts.isNamedImports(named) && !ts.isNamedExports(named)) return [];
  const from = spec && ts.isStringLiteral(spec) ? spec.text : "";
  return named.elements.map((e): [string, { spec: string; name: string }] => [
    e.name.text,
    { spec: from, name: (e.propertyName ?? e.name).text },
  ]);
}

const bound = new WeakMap<
  ts.SourceFile,
  Map<string, { spec: string; name: string }>
>();

export function bindings(source: ts.SourceFile) {
  const known =
    bound.get(source) ?? new Map(source.statements.flatMap(namedIn));
  bound.set(source, known);
  return known;
}

const declared = new WeakMap<ts.SourceFile, Map<string, ts.Node>>();

// The file's top-level functions and constants: the only ones a name alone can
// mean. A nested one is read from its scope, by `definitionOf`.
export function declarations(source: ts.SourceFile) {
  const known = declared.get(source);
  if (known) return known;
  const out = new Map<string, ts.Node>();
  for (const s of source.statements) {
    const named = ts.isVariableStatement(s)
      ? s.declarationList.declarations
      : [s];
    for (const n of named) {
      const fn = ts.isFunctionDeclaration(n) || ts.isVariableDeclaration(n);
      if (fn && n.name && ts.isIdentifier(n.name)) out.set(n.name.text, n);
    }
  }
  declared.set(source, out);
  return out;
}
