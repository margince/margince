import { readFileSync } from "node:fs";
import ts from "typescript";
import { bindingOf, declarationOf } from "../src/testing/classnames";
import { parseSource, resolveRelative } from "./lib/source-tree";

/** `X * 4` and `X + 3000`, the two ways a budget here is built from another. */
function arithmetic(
  node: ts.BinaryExpression,
  value: (node: ts.Expression) => number | undefined,
): number | undefined {
  const left = value(node.left);
  const right = value(node.right);
  if (left === undefined || right === undefined) return undefined;
  if (node.operatorToken.kind === ts.SyntaxKind.AsteriskToken)
    return left * right;
  if (node.operatorToken.kind === ts.SyntaxKind.PlusToken) return left + right;
  return undefined;
}

/**
 * A resolver for numbers: a literal, a name holding one in the scope it is
 * read from, or arithmetic over those. A name the file never binds takes its
 * value from `imported`. One that does not resolve is reported, not guessed at.
 */
export function resolver(imported: ReadonlyMap<string, number>) {
  const value = (
    node: ts.Expression,
    reading: ReadonlySet<ts.Node> = new Set(),
  ): number | undefined => {
    if (ts.isNumericLiteral(node)) return Number(node.text);
    if (ts.isIdentifier(node)) return named(node, reading);
    if (ts.isParenthesizedExpression(node))
      return value(node.expression, reading);
    if (ts.isBinaryExpression(node))
      return arithmetic(node, (side) => value(side, reading));
    return undefined;
  };
  // `reading` holds the declarations on the way here, so `const A = B` beside
  // `const B = A` ends unresolved instead of looping.
  const named = (
    name: ts.Identifier,
    reading: ReadonlySet<ts.Node>,
  ): number | undefined => {
    const bound = bindingOf(name);
    if (bound === undefined) return imported.get(name.text);
    const declared = declarationOf(name);
    if (!declared || !ts.isVariableDeclaration(declared)) return undefined;
    if (!declared.initializer || reading.has(declared)) return undefined;
    return value(declared.initializer, new Set([...reading, declared]));
  };
  return value;
}

export function parse(file: string): ts.SourceFile {
  return parseSource(file, readFileSync(file, "utf8"));
}

/** The numbers a module exports at its top level. */
function exportedConsts(source: ts.SourceFile): Map<string, number> {
  const known = new Map<string, number>();
  const value = resolver(new Map());
  for (const statement of source.statements) {
    if (!ts.isVariableStatement(statement)) continue;
    const exported = statement.modifiers?.some(
      (modifier) => modifier.kind === ts.SyntaxKind.ExportKeyword,
    );
    if (!exported) continue;
    for (const declaration of statement.declarationList.declarations) {
      if (!ts.isIdentifier(declaration.name) || !declaration.initializer)
        continue;
      const resolved = value(declaration.initializer);
      if (resolved !== undefined) known.set(declaration.name.text, resolved);
    }
  }
  return known;
}

/** The numeric constants one named import brings in, added to `into`. */
function takeImport(
  statement: ts.ImportDeclaration,
  file: string,
  into: Map<string, number>,
): void {
  if (!ts.isStringLiteral(statement.moduleSpecifier)) return;
  const bindings = statement.importClause?.namedBindings;
  if (!bindings || !ts.isNamedImports(bindings)) return;
  const target = resolveRelative(file, statement.moduleSpecifier.text);
  if (!target) return;
  const theirs = exportedConsts(parse(target));
  for (const element of bindings.elements) {
    const value = theirs.get((element.propertyName ?? element.name).text);
    if (value !== undefined) into.set(element.name.text, value);
  }
}

/**
 * The numeric constants a test imports from a sibling module.
 * `{ timeout: READ_POLL_MS * EXPECTED_READS * 5 }` is a real budget, and
 * READ_POLL_MS lives in the hook it drives. One hop only: a budget two modules
 * away is reported unresolved, which is the safe end of that trade.
 */
export function importedConsts(
  source: ts.SourceFile,
  file: string,
): Map<string, number> {
  const imported = new Map<string, number>();
  for (const statement of source.statements) {
    if (ts.isImportDeclaration(statement))
      takeImport(statement, file, imported);
  }
  return imported;
}
