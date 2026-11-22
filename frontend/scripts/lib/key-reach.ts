// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// WHICH CATALOG KEYS THE SHIPPED BUNDLE RENDERS, by symbol rather than module.
//
// bundle-reach says which modules load. A loaded module can still hold an
// export nothing renders, and a key that export names is not rendered either.
// This walks from what runs at load (statements that are not inert
// declarations, and every export of an entry) through the symbols those
// statements name, using the type checker to follow imports and re-exports.
//
// A template key is read with its types: `a.${kind}` vouches only for the keys
// the literal union of `kind` can produce, and for every key under `a.` when
// `kind` is a plain string.

import ts from "typescript";
import {
  globTargets,
  isKeyLiteral,
  type Module,
  metaGlobPatterns,
} from "./bundle-reach";

export type KeyUse = {
  readonly literals: ReadonlySet<string>;
  /** One pattern per template that can produce a key. */
  readonly templates: readonly RegExp[];
  /** Loaded modules the checked program does not hold; their keys are unread. */
  readonly missing: readonly string[];
};

type Unit = {
  readonly node: ts.Node;
  readonly exported: boolean;
  /** A declaration whose evaluation does nothing until something names it. */
  readonly inert: boolean;
};

// Calls that only build a component or a context; nothing renders until used.
const LAZY_CALLS = new Set(["memo", "forwardRef", "lazy", "createContext"]);

// Same dot rule as bundle-reach: without one a head is a class name or a URL.
const KEY_STEM = /^[A-Za-z0-9_]+\.[A-Za-z0-9_.]*$/;

// A union wider than this is a wildcard; it is a string in all but name.
const MAX_ALTERNATIVES = 2000;

const hasExport = (node: ts.Node): boolean =>
  ts.canHaveModifiers(node) &&
  (ts.getModifiers(node) ?? []).some(
    (modifier) => modifier.kind === ts.SyntaxKind.ExportKeyword,
  );

function isPure(expression: ts.Expression | undefined): boolean {
  if (expression === undefined) return true;
  if (
    ts.isArrowFunction(expression) ||
    ts.isFunctionExpression(expression) ||
    ts.isClassExpression(expression) ||
    ts.isIdentifier(expression) ||
    ts.isLiteralExpression(expression) ||
    ts.isJsxElement(expression) ||
    ts.isJsxSelfClosingElement(expression) ||
    ts.isJsxFragment(expression) ||
    expression.kind === ts.SyntaxKind.TrueKeyword ||
    expression.kind === ts.SyntaxKind.FalseKeyword ||
    expression.kind === ts.SyntaxKind.NullKeyword
  ) {
    return true;
  }
  if (
    ts.isParenthesizedExpression(expression) ||
    ts.isAsExpression(expression) ||
    ts.isSatisfiesExpression(expression) ||
    ts.isNonNullExpression(expression) ||
    ts.isPropertyAccessExpression(expression)
  ) {
    return isPure(expression.expression);
  }
  if (ts.isArrayLiteralExpression(expression)) {
    return expression.elements.every(isPure);
  }
  if (ts.isObjectLiteralExpression(expression)) {
    return expression.properties.every((property) =>
      ts.isPropertyAssignment(property)
        ? isPure(property.initializer)
        : ts.isSpreadAssignment(property)
          ? isPure(property.expression)
          : true,
    );
  }
  if (ts.isCallExpression(expression)) {
    const callee = ts.isPropertyAccessExpression(expression.expression)
      ? expression.expression.name
      : expression.expression;
    return (
      ts.isIdentifier(callee) &&
      LAZY_CALLS.has(callee.text) &&
      expression.arguments.every(isPure)
    );
  }
  return false;
}

const hasDecorators = (node: ts.Node): boolean =>
  ts.canHaveDecorators(node) && (ts.getDecorators(node) ?? []).length > 0;

// A class runs its decorators, its `extends` expression, its computed member
// names and its static initializers when it is defined.
function isInertClass(declaration: ts.ClassDeclaration): boolean {
  const extendsExpressions = (declaration.heritageClauses ?? [])
    .filter((clause) => clause.token === ts.SyntaxKind.ExtendsKeyword)
    .flatMap((clause) => clause.types.map((type) => type.expression));
  return (
    !hasDecorators(declaration) &&
    extendsExpressions.every(isPure) &&
    declaration.members.every((member) => {
      const isStatic =
        ts.getCombinedModifierFlags(member) & ts.ModifierFlags.Static;
      return (
        !hasDecorators(member) &&
        !ts.isClassStaticBlockDeclaration(member) &&
        !(ts.isPropertyDeclaration(member) && isStatic) &&
        !(member.name && ts.isComputedPropertyName(member.name))
      );
    })
  );
}

// An enum member's initializer and a namespace body are code too.
function isInertDeclaration(statement: ts.Statement): boolean {
  if (ts.isFunctionDeclaration(statement)) return true;
  if (ts.isClassDeclaration(statement)) return isInertClass(statement);
  if (ts.isEnumDeclaration(statement)) {
    return statement.members.every((member) => isPure(member.initializer));
  }
  return (
    ts.isModuleDeclaration(statement) &&
    (ts.getCombinedModifierFlags(statement) & ts.ModifierFlags.Ambient) !== 0
  );
}

function unitsOf(file: ts.SourceFile): Unit[] {
  return file.statements.flatMap((statement): Unit[] => {
    if (ts.isImportDeclaration(statement)) return [];
    if (ts.isInterfaceDeclaration(statement)) return [];
    if (ts.isTypeAliasDeclaration(statement)) return [];
    if (ts.isVariableStatement(statement)) {
      return statement.declarationList.declarations.map((declaration) => ({
        node: declaration,
        exported: hasExport(statement),
        inert: isPure(declaration.initializer),
      }));
    }
    const exportsOnly =
      ts.isExportDeclaration(statement) || ts.isExportAssignment(statement);
    return [
      {
        node: statement,
        exported: exportsOnly || hasExport(statement),
        inert: exportsOnly || isInertDeclaration(statement),
      },
    ];
  });
}

/** The statement or declarator at the top of a file that holds `node`. */
function unitNodeOf(node: ts.Node): ts.Node | undefined {
  for (let at = node; at.parent !== undefined; at = at.parent) {
    if (ts.isSourceFile(at.parent)) return at;
    if (
      ts.isVariableDeclarationList(at.parent) &&
      ts.isSourceFile(at.parent.parent.parent)
    ) {
      return at;
    }
  }
  return undefined;
}

const escaped = (text: string): string =>
  text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/** The strings a type can be, or null when it is wider than a finite union. */
function finiteStrings(
  checker: ts.TypeChecker,
  type: ts.Type,
): string[] | null {
  if (type.isStringLiteral()) return [type.value];
  if (type.isNumberLiteral()) return [String(type.value)];
  if (type.flags & ts.TypeFlags.BooleanLiteral) {
    return [checker.typeToString(type)];
  }
  if (!type.isUnion()) return null;
  const parts = type.types.map((member) => finiteStrings(checker, member));
  const all = parts.every((part) => part !== null);
  return all && parts.length <= MAX_ALTERNATIVES ? parts.flat() : null;
}

function templatePattern(
  node: ts.TemplateExpression,
  checker: ts.TypeChecker,
): RegExp {
  const spans = node.templateSpans.map((span) => {
    const values = finiteStrings(
      checker,
      checker.getTypeAtLocation(span.expression),
    );
    const choice =
      values === null || values.length === 0
        ? ".*"
        : `(?:${values.map(escaped).join("|")})`;
    return `${choice}${escaped(span.literal.text)}`;
  });
  return new RegExp(`^${escaped(node.head.text)}${spans.join("")}$`);
}

// Naming a member, a label or a JSX attribute asks the checker for the type of
// what it hangs off; only names that a scope lookup resolves are references.
function isReference(node: ts.Identifier): boolean {
  const parent = node.parent;
  const named =
    (ts.isPropertyAccessExpression(parent) && parent.name === node) ||
    (ts.isQualifiedName(parent) && parent.right === node) ||
    (ts.isJsxAttribute(parent) && parent.name === node) ||
    (ts.isBindingElement(parent) && parent.propertyName === node);
  const member =
    (ts.isPropertyAssignment(parent) ||
      ts.isMethodDeclaration(parent) ||
      ts.isPropertyDeclaration(parent) ||
      ts.isEnumMember(parent)) &&
    parent.name === node;
  return !named && !member;
}

class Reach {
  private readonly checker: ts.TypeChecker;
  private readonly unitsByFile = new Map<string, Unit[]>();
  private readonly unitByNode = new Map<ts.Node, Unit>();
  private readonly fullModules = new Set<string>();
  private readonly live = new Set<Unit>();
  private readonly queue: Unit[] = [];
  readonly literals = new Set<string>();
  readonly missing: string[] = [];
  readonly templates = new Map<string, RegExp>();

  constructor(
    private readonly program: ts.Program,
    private readonly loaded: ReadonlyMap<string, Module>,
    private readonly catalogs: ReadonlySet<string>,
  ) {
    this.checker = program.getTypeChecker();
  }

  run(entries: readonly string[]): void {
    for (const path of this.loaded.keys()) {
      for (const unit of this.unitsIn(path)) {
        if (!unit.inert) this.markUnit(unit);
      }
    }
    for (const entry of entries) this.markModule(entry);
    for (let unit = this.queue.pop(); unit; unit = this.queue.pop()) {
      this.visit(unit.node, unit.node.getSourceFile().fileName);
    }
  }

  private unitsIn(path: string): Unit[] {
    const known = this.unitsByFile.get(path);
    if (known !== undefined) return known;
    const file = this.program.getSourceFile(path);
    const reads = this.loaded.has(path) && !this.catalogs.has(path);
    if (reads && file === undefined) this.missing.push(path);
    const units = file === undefined || !reads ? [] : unitsOf(file);
    this.unitsByFile.set(path, units);
    for (const unit of units) this.unitByNode.set(unit.node, unit);
    return units;
  }

  private markUnit(unit: Unit): void {
    if (this.live.has(unit)) return;
    this.live.add(unit);
    this.queue.push(unit);
  }

  private markModule(path: string): void {
    if (this.fullModules.has(path)) return;
    this.fullModules.add(path);
    for (const unit of this.unitsIn(path)) {
      if (unit.exported) this.markUnit(unit);
    }
  }

  private markNamed(declaration: ts.Node): void {
    if (ts.isSourceFile(declaration)) {
      this.markModule(declaration.fileName);
      return;
    }
    const top = unitNodeOf(declaration);
    if (top === undefined) return;
    this.unitsIn(declaration.getSourceFile().fileName);
    const unit = this.unitByNode.get(top);
    if (unit !== undefined) this.markUnit(unit);
  }

  private markSymbol(symbol: ts.Symbol | undefined): void {
    const target =
      symbol !== undefined && symbol.flags & ts.SymbolFlags.Alias
        ? this.checker.getAliasedSymbol(symbol)
        : symbol;
    for (const declaration of target?.declarations ?? []) {
      this.markNamed(declaration);
    }
  }

  private markSpecifier(specifier: ts.Expression): void {
    const module = this.checker.getSymbolAtLocation(specifier);
    for (const declaration of module?.declarations ?? []) {
      this.markNamed(declaration);
    }
  }

  private visit(node: ts.Node, path: string): void {
    if (ts.isExpressionWithTypeArguments(node)) {
      this.visit(node.expression, path);
      return;
    }
    if (ts.isTypeNode(node)) return;
    this.note(node, path);
    if (ts.isExportSpecifier(node)) {
      this.markSymbol(this.checker.getExportSpecifierLocalTargetSymbol(node));
      return;
    }
    ts.forEachChild(node, (child) => this.visit(child, path));
  }

  private note(node: ts.Node, path: string): void {
    if (ts.isIdentifier(node)) {
      if (isReference(node)) this.reference(node);
    } else if (isKeyLiteral(node)) {
      this.literals.add(node.text);
    } else if (ts.isTemplateExpression(node)) {
      this.template(node);
    } else if (ts.isCallExpression(node)) {
      this.call(node, path);
    } else if (ts.isExportDeclaration(node) && node.moduleSpecifier) {
      // `export *` and `export * as ns` hand every export on; a named list
      // is followed name by name at its specifiers.
      const named =
        node.exportClause && !ts.isNamespaceExport(node.exportClause);
      if (!named) this.markSpecifier(node.moduleSpecifier);
    }
  }

  private reference(node: ts.Identifier): void {
    const parent = node.parent;
    this.markSymbol(
      ts.isShorthandPropertyAssignment(parent)
        ? this.checker.getShorthandAssignmentValueSymbol(parent)
        : this.checker.getSymbolAtLocation(node),
    );
  }

  private template(node: ts.TemplateExpression): void {
    if (!KEY_STEM.test(node.head.text)) return;
    const pattern = templatePattern(node, this.checker);
    this.templates.set(pattern.source, pattern);
  }

  private call(node: ts.CallExpression, path: string): void {
    const [argument] = node.arguments;
    if (node.expression.kind === ts.SyntaxKind.ImportKeyword && argument) {
      this.markSpecifier(argument);
    }
    const patterns = metaGlobPatterns(node);
    if (patterns === undefined) return;
    for (const target of globTargets(this.loaded.keys(), path, patterns)) {
      this.markModule(target);
    }
  }
}

/**
 * The catalog use of every module in `loaded` that an entry's code reaches.
 * `entries` run in full, as do modules loaded by `import()`, a glob or a
 * namespace import, whose exports are read by name at run time.
 */
export function liveKeyUse(
  program: ts.Program,
  loaded: ReadonlyMap<string, Module>,
  entries: readonly string[],
  catalogs: ReadonlySet<string>,
): KeyUse {
  const reach = new Reach(program, loaded, catalogs);
  reach.run(entries);
  return {
    literals: reach.literals,
    templates: [...reach.templates.values()],
    missing: reach.missing,
  };
}

/** Whether the code names `key` literally or a template can produce it. */
export function usesKey(use: KeyUse, key: string): boolean {
  return use.literals.has(key) || use.templates.some((p) => p.test(key));
}
