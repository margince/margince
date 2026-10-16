import { readFileSync } from "node:fs";
import ts from "typescript";
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
 * A resolver for numbers written in this file: a literal, a name it gave a
 * number to, or arithmetic over those. A name imported from elsewhere does not
 * resolve, and that is reported rather than guessed at.
 */
export function resolver(consts: Map<string, number>) {
  const value = (node: ts.Expression): number | undefined => {
    if (ts.isNumericLiteral(node)) return Number(node.text);
    if (ts.isIdentifier(node)) return consts.get(node.text);
    if (ts.isParenthesizedExpression(node)) return value(node.expression);
    if (ts.isBinaryExpression(node)) return arithmetic(node, value);
    return undefined;
  };
  return value;
}

/** `const X = 5000` and `const Y = X * 4` anywhere in one file. */
function declaredConsts(
  source: ts.SourceFile,
  seeded: Map<string, number>,
): Map<string, number> {
  // Every value each name is ever given, not the last one. This reader has no
  // scope information: two `describe` blocks may each declare `SETTLE_MS`, or a
  // function-local may shadow a module-level one.
  // A name given two different values resolves to nothing, and a waiter that
  // needed it is reported rather than folded to the wrong number.
  const seen = new Map<string, Set<number>>();
  for (const [name, resolved] of seeded) seen.set(name, new Set([resolved]));
  const single = (): Map<string, number> => {
    const settled = new Map<string, number>();
    for (const [name, values] of seen) {
      const only = values.size === 1 ? [...values][0] : undefined;
      if (only !== undefined) settled.set(name, only);
    }
    return settled;
  };
  const visit = (node: ts.Node): void => {
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.initializer
    ) {
      const resolved = resolver(single())(node.initializer);
      if (resolved !== undefined) {
        const values = seen.get(node.name.text);
        if (values) values.add(resolved);
        else seen.set(node.name.text, new Set([resolved]));
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return single();
}

export function parse(file: string): ts.SourceFile {
  return parseSource(file, readFileSync(file, "utf8"));
}

/**
 * The numbers a module exports at its top level. Not `declaredConsts`, which
 * walks a whole file flat. There, a function-local `const POLL_MS = 1` would
 * shadow the exported `POLL_MS = 5000` and fold a 10000ms budget to 2ms.
 */
function exportedConsts(source: ts.SourceFile): Map<string, number> {
  const known = new Map<string, number>();
  const value = resolver(known);
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
 * A test's numeric constants, including the ones it imports from a sibling
 * module. `{ timeout: READ_POLL_MS * EXPECTED_READS * 5 }` is a real budget,
 * and READ_POLL_MS lives in the hook it drives. One hop only: a budget two
 * modules away is reported unresolved, which is the safe end of that trade.
 */
export function numericConsts(
  source: ts.SourceFile,
  file: string,
): Map<string, number> {
  const imported = new Map<string, number>();
  for (const statement of source.statements) {
    if (ts.isImportDeclaration(statement))
      takeImport(statement, file, imported);
  }
  return declaredConsts(source, imported);
}
