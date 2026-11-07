import ts from "typescript";
import { parseSource } from "./source-tree.ts";

function exportsValue(statement) {
  return (
    ts.isExportDeclaration(statement) ||
    ts.isExportAssignment(statement) ||
    statement.modifiers?.some(
      (modifier) => modifier.kind === ts.SyntaxKind.ExportKeyword,
    )
  );
}

function startupImport(statement) {
  if (!ts.isExpressionStatement(statement)) return null;
  const expression = ts.isVoidExpression(statement.expression)
    ? statement.expression.expression
    : statement.expression;
  if (
    !ts.isCallExpression(expression) ||
    expression.expression.kind !== ts.SyntaxKind.ImportKeyword
  )
    return null;
  const [specifier] = expression.arguments;
  return specifier && ts.isStringLiteral(specifier) ? specifier.text : null;
}

// Follow only top-level bootstrap imports. Lazy imports inside components still
// need stories, and a module exporting UI is never a startup-only exclusion.
export function startupModules(entry, read, resolve) {
  const modules = new Set();
  function visit(file) {
    if (!file || modules.has(file)) return;
    const source = parseSource(file, read(file));
    if (source.statements.some(exportsValue)) return;
    modules.add(file);
    for (const statement of source.statements) {
      const specifier = startupImport(statement);
      if (specifier !== null) visit(resolve(file, specifier));
    }
  }
  visit(entry);
  return modules;
}
