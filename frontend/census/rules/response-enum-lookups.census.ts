// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import ts from "typescript";
import {
  type CensusFile,
  type CensusRule,
  frontendRoot,
  repoRoot,
} from "../tree";

// A response enum is open, but its generated union is closed. So
// `LABEL[deal.status]` type-checks and reads `undefined` for a value a newer
// server sends. docs/explanation/frontend-architecture.md states the rule.

// A response enum is any marked enum in crm.yaml; openresponseenums_test.go
// holds the markers. A narrowed subset counts: `if (s !== "won") LABEL[s]`.
export const contractPath = join(repoRoot, "backend", "api", "crm.yaml");

const MARKER = /^(\s*)(?:-\s+)?.*\bx-extensible-enum:\s*true\b/;

const indentOf = (line: string) => line.length - line.trimStart().length;
const isTrivia = (line: string) => /^\s*(#.*)?$/.test(line);

/** The values inside a flow sequence's brackets: `a, 'b', null`. */
function flowValues(inside: string): string[] {
  return inside
    .split(",")
    .map((raw) => raw.trim().replace(/^(['"])(.*)\1$/, "$2"))
    .filter((value) => value !== "" && value !== "null" && value !== "~");
}

/** The text from the `[` at `start` to its matching `]`, across lines. */
function bracketed(lines: string[], line: number, start: number): string {
  let depth = 0;
  let out = "";
  for (let at = line; at < lines.length; at++) {
    const text = lines[at] ?? "";
    for (let col = at === line ? start : 0; col < text.length; col++) {
      const ch = text[col];
      if (ch === "[") depth++;
      else if (ch === "]" && --depth === 0) return out;
      else if (depth > 0) out += ch;
    }
    out += " ";
  }
  throw new Error(`crm.yaml:${line + 1} an enum list never closes`);
}

/** The values of the `enum:` key at `line`, flow or block. */
function enumValuesAt(lines: string[], line: number): string[] {
  const text = lines[line] ?? "";
  const open = text.indexOf("[", text.indexOf("enum:"));
  if (open >= 0) return flowValues(bracketed(lines, line, open));
  const values: string[] = [];
  const keyIndent = indentOf(text);
  for (let at = line + 1; at < lines.length; at++) {
    const item = lines[at] ?? "";
    if (isTrivia(item)) continue;
    // A flow list may open on the line below its key.
    if (values.length === 0 && /^\s*\[/.test(item)) {
      return flowValues(bracketed(lines, at, item.indexOf("[")));
    }
    const dash = /^(\s*)-\s+(.*)$/.exec(item);
    if (!dash || (dash[1] ?? "").length < keyIndent) break;
    values.push(...flowValues(dash[2] ?? ""));
  }
  return values;
}

/**
 * The key of the block mapping a marker line sits in: every line from the
 * mapping's first line to its last at the marker's indent.
 */
function enumKeyInMapping(lines: string[], marker: number): number {
  const indent = indentOf(lines[marker] ?? "");
  const inMapping = (at: number) => {
    const text = lines[at] ?? "";
    return isTrivia(text) || indentOf(text) >= indent;
  };
  let first = marker;
  while (first > 0 && inMapping(first - 1)) first--;
  for (let at = first; at < lines.length && inMapping(at); at++) {
    const text = lines[at] ?? "";
    if (indentOf(text) === indent && /^\s*enum:/.test(text)) return at;
  }
  throw new Error(`crm.yaml:${marker + 1} a marker with no enum beside it`);
}

export type ResponseEnums = Readonly<{
  /** Each marked enum's values, null left out. */
  sets: readonly ReadonlySet<string>[];
  /** The contract line of each set's marker, in the same order. */
  lines: readonly number[];
  /** How many markers the contract holds outside its comments. */
  markers: number;
}>;

/** Every marked enum in a contract's text, read from its markers. */
export function responseEnums(contract: string): ResponseEnums {
  const lines = contract.split(/\r?\n/);
  const sets: Set<string>[] = [];
  const markerLines: number[] = [];
  let markers = 0;
  lines.forEach((text, at) => {
    const code = text.replace(/\s#.*$/, "");
    if (/^\s*#/.test(text) || !MARKER.test(code)) return;
    markers++;
    const inline = code.lastIndexOf("enum:", code.indexOf("x-extensible-enum"));
    // `enum:` here is the key itself, not the tail of `x-extensible-enum:`.
    const flow = inline >= 0 && /[\s{,]$/.test(code.slice(0, inline));
    const values = flow
      ? flowValues(bracketed(lines, at, code.indexOf("[", inline)))
      : enumValuesAt(lines, enumKeyInMapping(lines, at));
    sets.push(new Set(values));
    markerLines.push(at + 1);
  });
  return { sets, lines: markerLines, markers };
}

/** The string literals a type is a union of, `undefined` and `null` set
 * aside; undefined when it is anything else. */
function literalsOf(type: ts.Type): string[] | undefined {
  const out: string[] = [];
  for (const part of type.isUnion() ? type.types : [type]) {
    if (part.flags & (ts.TypeFlags.Undefined | ts.TypeFlags.Null)) continue;
    if (!part.isStringLiteral()) return undefined;
    out.push(part.value);
  }
  return out;
}

const CONTRACT_FILE = /[\\/]src[\\/]api[\\/]/;

// The language's own declarations, as `Array.prototype.map`.
const LIBRARY_FILE = /[\\/]typescript[\\/]lib[\\/]lib\.[^\\/]*\.d\.ts$/;

const KEEPS_SOURCE = new Set([
  "Array",
  "Exclude",
  "Extract",
  "NonNullable",
  "NoInfer",
  "Readonly",
  "ReadonlyArray",
  "ReadonlySet",
  "Set",
]);

const inContract = (node: ts.Node) =>
  CONTRACT_FILE.test(node.getSourceFile().fileName);

/**
 * Where a value's type comes from, followed back to the generated contract.
 * A union whose words match a response enum is not enough. The app has its
 * own vocabularies, such as its record kinds, that share those words.
 */
class Origin {
  // Each answer once. A node met again while its answer is open is a cycle,
  // and a cycle alone proves no contract source.
  private readonly known = new Map<ts.Node, boolean>();

  constructor(private readonly checker: ts.TypeChecker) {}

  private memo(node: ts.Node, answer: () => boolean): boolean {
    const known = this.known.get(node);
    if (known !== undefined) return known;
    this.known.set(node, false);
    const found = answer();
    this.known.set(node, found);
    return found;
  }

  /** Whether an expression's value is read from the contract. */
  expression(node: ts.Expression): boolean {
    return this.memo(node, () => this.expressionOnce(node));
  }

  private expressionOnce(node: ts.Expression): boolean {
    if (
      ts.isParenthesizedExpression(node) ||
      ts.isNonNullExpression(node) ||
      ts.isSatisfiesExpression(node)
    ) {
      return this.expression(node.expression);
    }
    if (ts.isAsExpression(node) || ts.isTypeAssertionExpression(node)) {
      return this.typeNode(node.type);
    }
    if (ts.isConditionalExpression(node)) {
      return this.expression(node.whenTrue) || this.expression(node.whenFalse);
    }
    if (ts.isBinaryExpression(node)) {
      return this.expression(node.left) || this.expression(node.right);
    }
    if (ts.isTemplateExpression(node)) {
      return node.templateSpans.some((span) =>
        this.expression(span.expression),
      );
    }
    if (ts.isPropertyAccessExpression(node)) {
      return this.symbol(this.checker.getSymbolAtLocation(node.name));
    }
    if (ts.isElementAccessExpression(node)) {
      // An item of a contract list is the contract's; a value of a map keyed
      // by a contract value is the map's own.
      const list = this.checker.getTypeAtLocation(node.expression);
      return (
        this.symbol(this.checker.getSymbolAtLocation(node)) ||
        ((this.checker.isArrayType(list) || this.checker.isTupleType(list)) &&
          this.expression(node.expression))
      );
    }
    if (ts.isIdentifier(node)) {
      return this.symbol(this.checker.getSymbolAtLocation(node));
    }
    if (ts.isCallExpression(node)) return this.call(node);
    return false;
  }

  /**
   * A call's result: its declared return type. A method of the language's own
   * library, such as `Array.map`, also passes on the list it is called on and
   * what its callbacks return.
   */
  private call(node: ts.CallExpression): boolean {
    const declared = this.checker.getResolvedSignature(node)?.declaration;
    if (!declared || ts.isJSDocSignature(declared)) return false;
    if (inContract(declared)) return true;
    if (declared.type && this.typeNode(declared.type)) return true;
    if (!LIBRARY_FILE.test(declared.getSourceFile().fileName)) return false;
    const callee = node.expression;
    if (
      ts.isPropertyAccessExpression(callee) &&
      this.expression(callee.expression)
    ) {
      return true;
    }
    return node.arguments.some(
      (argument) =>
        (ts.isArrowFunction(argument) || ts.isFunctionExpression(argument)) &&
        this.returned(argument),
    );
  }

  /** Whether a callback returns a value read from the contract. */
  private returned(fn: ts.ArrowFunction | ts.FunctionExpression): boolean {
    if (!ts.isBlock(fn.body)) return this.expression(fn.body);
    let found = false;
    const visit = (child: ts.Node) => {
      if (found || (child !== fn.body && ts.isFunctionLike(child))) return;
      if (ts.isReturnStatement(child) && child.expression) {
        found = this.expression(child.expression);
      }
      ts.forEachChild(child, visit);
    };
    visit(fn.body);
    return found;
  }

  /** Whether a declared name holds a value read from the contract. */
  symbol(found: ts.Symbol | undefined): boolean {
    if (!found) return false;
    const named =
      found.flags & ts.SymbolFlags.Alias
        ? this.checker.getAliasedSymbol(found)
        : found;
    return (named.declarations ?? []).some((declared) =>
      this.declaration(declared),
    );
  }

  private declaration(node: ts.Declaration): boolean {
    return this.memo(node, () => this.declarationOnce(node));
  }

  private declarationOnce(node: ts.Declaration): boolean {
    if (inContract(node)) return true;
    if (ts.isBindingElement(node)) return this.binding(node);
    if (
      ts.isParameter(node) ||
      ts.isVariableDeclaration(node) ||
      ts.isPropertySignature(node) ||
      ts.isPropertyDeclaration(node)
    ) {
      if (node.type && this.typeNode(node.type)) return true;
      if (ts.isParameter(node)) return this.parameter(node);
      if (ts.isPropertySignature(node) || node.initializer === undefined) {
        return false;
      }
      return this.expression(node.initializer);
    }
    if (ts.isPropertyAssignment(node)) return this.expression(node.initializer);
    if (ts.isShorthandPropertyAssignment(node)) {
      return this.symbol(this.checker.getShorthandAssignmentValueSymbol(node));
    }
    return false;
  }

  /** `const { status } = deal`: the property the name reads. */
  private binding(node: ts.BindingElement): boolean {
    const pattern = node.parent;
    const name = node.propertyName ?? node.name;
    if (!ts.isObjectBindingPattern(pattern) || !ts.isIdentifier(name)) {
      return false;
    }
    const whole = this.checker.getTypeAtLocation(pattern);
    return this.symbol(whole.getProperty(name.text));
  }

  /**
   * A parameter with no contract type. It may be a callback over a contract
   * list, as in `deal.tags.map((tag) => …)`. It may be a helper's in the same
   * file that a caller hands a contract value.
   */
  private parameter(node: ts.ParameterDeclaration): boolean {
    const fn = node.parent;
    const at = fn.parameters.indexOf(node);
    const call = fn.parent;
    if (
      ts.isCallExpression(call) &&
      call.arguments.includes(fn as ts.Expression) &&
      ts.isPropertyAccessExpression(call.expression)
    ) {
      return this.expression(call.expression.expression);
    }
    const name = helperName(fn);
    if (!name) return false;
    const target = this.checker.getSymbolAtLocation(name);
    let found = false;
    const visit = (child: ts.Node) => {
      if (found) return;
      if (
        ts.isCallExpression(child) &&
        this.checker.getSymbolAtLocation(child.expression) === target
      ) {
        const argument = child.arguments[at];
        found = argument !== undefined && this.expression(argument);
      }
      ts.forEachChild(child, visit);
    };
    visit(node.getSourceFile());
    return found;
  }

  /** Whether a type, as written, names a type of the contract. */
  typeNode(node: ts.TypeNode): boolean {
    return this.memo(node, () => this.typeNodeOnce(node));
  }

  private typeNodeOnce(node: ts.TypeNode): boolean {
    if (ts.isIndexedAccessTypeNode(node) || ts.isArrayTypeNode(node)) {
      return this.typeNode(
        ts.isArrayTypeNode(node) ? node.elementType : node.objectType,
      );
    }
    if (ts.isParenthesizedTypeNode(node) || ts.isTypeOperatorNode(node)) {
      return this.typeNode(node.type);
    }
    if (ts.isUnionTypeNode(node)) {
      return node.types.some((member) => this.typeNode(member));
    }
    if (!ts.isTypeReferenceNode(node)) return false;
    const found = this.checker.getSymbolAtLocation(node.typeName);
    const named =
      found && found.flags & ts.SymbolFlags.Alias
        ? this.checker.getAliasedSymbol(found)
        : found;
    const declared = named?.declarations?.[0];
    if (!declared) return false;
    if (inContract(declared)) return true;
    if (ts.isTypeAliasDeclaration(declared) && this.typeNode(declared.type)) {
      return true;
    }
    // A library wrapper such as NonNullable<…> or Array<…> keeps its
    // argument's source. Record<K, V> does not: its values are not its keys.
    return (
      LIBRARY_FILE.test(declared.getSourceFile().fileName) &&
      KEEPS_SOURCE.has(named?.name ?? "") &&
      (node.typeArguments ?? []).some((argument) => this.typeNode(argument))
    );
  }
}

function helperName(fn: ts.SignatureDeclaration): ts.Identifier | undefined {
  if (ts.isFunctionDeclaration(fn)) return fn.name;
  const holder = fn.parent;
  return ts.isVariableDeclaration(holder) && ts.isIdentifier(holder.name)
    ? holder.name
    : undefined;
}

/** Whether a type's string literals all belong to one marked enum. */
function inOneEnum(
  type: ts.Type,
  sets: readonly ReadonlySet<string>[],
): boolean {
  const literals = literalsOf(type);
  if (!literals || literals.length < 2) return false;
  return sets.some((set) => literals.every((value) => set.has(value)));
}

/**
 * What the rule asks of one value: is it a response enum value? Its type must
 * hold the words of one marked enum, and its source must be the contract.
 */
class ResponseValues {
  private readonly origin: Origin;

  constructor(
    private readonly checker: ts.TypeChecker,
    private readonly sets: readonly ReadonlySet<string>[],
  ) {
    this.origin = new Origin(checker);
  }

  /** Whether an expression holds a response enum value. */
  holds(node: ts.Expression): boolean {
    if (ts.isTemplateExpression(node)) {
      return node.templateSpans.some((span) => this.holds(span.expression));
    }
    if (
      inOneEnum(this.checker.getTypeAtLocation(node), this.sets) &&
      this.origin.expression(node)
    ) {
      return true;
    }
    return this.handedToHelper(node);
  }

  /** Whether the type of a switch's subject is a response enum. */
  enumType(node: ts.Expression): boolean {
    return (
      inOneEnum(this.checker.getTypeAtLocation(node), this.sets) &&
      this.origin.expression(node)
    );
  }

  /**
   * A parameter of a helper in the same file, typed loosely or generically,
   * that a caller hands a response enum value.
   */
  private handedToHelper(node: ts.Expression): boolean {
    if (!ts.isIdentifier(node)) return false;
    const declared = this.checker.getSymbolAtLocation(node)?.valueDeclaration;
    if (!declared || !ts.isParameter(declared)) return false;
    const name = helperName(declared.parent);
    if (!name) return false;
    const at = declared.parent.parameters.indexOf(declared);
    const target = this.checker.getSymbolAtLocation(name);
    let found = false;
    const visit = (child: ts.Node) => {
      if (found) return;
      if (
        ts.isCallExpression(child) &&
        this.checker.getSymbolAtLocation(child.expression) === target
      ) {
        const argument = child.arguments[at];
        found =
          argument !== undefined &&
          argument !== node &&
          inOneEnum(this.checker.getTypeAtLocation(argument), this.sets) &&
          this.origin.expression(argument);
      }
      ts.forEachChild(child, visit);
    };
    visit(node.getSourceFile());
    return found;
  }
}

const isWriteTarget = (node: ts.Node): boolean => {
  const parent = node.parent;
  return (
    ts.isBinaryExpression(parent) &&
    parent.left === node &&
    parent.operatorToken.kind >= ts.SyntaxKind.FirstAssignment &&
    parent.operatorToken.kind <= ts.SyntaxKind.LastAssignment
  );
};

/** Whether a type admits `undefined`, so the compiler already makes the
 * reader answer the missing case. `any` proves nothing. */
const admitsUndefined = (type: ts.Type) =>
  (type.isUnion() ? type.types : [type]).some(
    (part) => part.flags & ts.TypeFlags.Undefined,
  );

const EXITS = new Set([
  ts.SyntaxKind.ReturnStatement,
  ts.SyntaxKind.ThrowStatement,
]);

/**
 * Whether a switch over a response enum lets an unlisted value fall out. It
 * does when it has a case for every listed value and no `default`. Code after
 * a switch whose cases all return or throw answers any other value.
 */
function fallsOut(
  node: ts.SwitchStatement,
  checker: ts.TypeChecker,
  values: ResponseValues,
): boolean {
  const clauses = node.caseBlock.clauses;
  if (clauses.some(ts.isDefaultClause)) return false;
  const literals = literalsOf(checker.getTypeAtLocation(node.expression));
  if (!literals || !values.enumType(node.expression)) return false;
  const cased = new Set(
    clauses.flatMap((clause) =>
      ts.isCaseClause(clause) && ts.isStringLiteralLike(clause.expression)
        ? [clause.expression.text]
        : [],
    ),
  );
  if (!literals.every((value) => cased.has(value))) return false;
  const exits = clauses.every((clause) => {
    const last = clause.statements.at(-1);
    return last === undefined || EXITS.has(last.kind);
  });
  const block = node.parent;
  const followed =
    (ts.isBlock(block) || ts.isSourceFile(block) || ts.isCaseClause(block)) &&
    block.statements.at(-1) !== node;
  return !(exits && followed);
}

/** Whether a guard in front of a read proves the map holds the value:
 * `Object.hasOwn(M, v) ? M[v] : x` or `if (v in M) … M[v]`. */
function guarded(node: ts.ElementAccessExpression): boolean {
  const map = node.expression.getText();
  const key = node.argumentExpression.getText();
  const proves = (test: ts.Expression): boolean => {
    if (!ts.isBinaryExpression(test)) {
      return (
        ts.isCallExpression(test) &&
        test.expression.getText() === "Object.hasOwn" &&
        test.arguments[0]?.getText() === map &&
        test.arguments[1]?.getText() === key
      );
    }
    if (test.operatorToken.kind === ts.SyntaxKind.AmpersandAmpersandToken) {
      return proves(test.left) || proves(test.right);
    }
    return (
      test.operatorToken.kind === ts.SyntaxKind.InKeyword &&
      test.left.getText() === key &&
      test.right.getText() === map
    );
  };
  for (let at: ts.Node = node; at.parent; at = at.parent) {
    const parent = at.parent;
    if (ts.isFunctionLike(parent)) return false;
    let condition: ts.Expression | undefined;
    if (ts.isConditionalExpression(parent) && parent.whenTrue === at) {
      condition = parent.condition;
    } else if (ts.isIfStatement(parent) && parent.thenStatement === at) {
      condition = parent.expression;
    }
    if (condition && proves(condition)) return true;
  }
  return false;
}

/** Whether the code around a read answers a missing value: `M[v] ?? x`,
 * `M[v]?.x`, or a guard that the map holds `v`. */
function answeredInPlace(node: ts.ElementAccessExpression): boolean {
  const parent = node.parent;
  if (
    ts.isBinaryExpression(parent) &&
    parent.left === node &&
    parent.operatorToken.kind === ts.SyntaxKind.QuestionQuestionToken
  ) {
    return true;
  }
  if (
    (ts.isPropertyAccessExpression(parent) ||
      ts.isElementAccessExpression(parent) ||
      ts.isCallExpression(parent)) &&
    parent.questionDotToken !== undefined
  ) {
    return true;
  }
  return guarded(node);
}

/** Whether `node` reads a map by a response enum value and nothing answers a
 * value the map does not hold. */
function readsUnanswered(
  node: ts.Node,
  checker: ts.TypeChecker,
  values: ResponseValues,
): node is ts.ElementAccessExpression {
  return (
    ts.isElementAccessExpression(node) &&
    !ts.isLiteralExpression(node.argumentExpression) &&
    !isWriteTarget(node) &&
    !answeredInPlace(node) &&
    !admitsUndefined(checker.getTypeAtLocation(node)) &&
    values.holds(node.argumentExpression)
  );
}

/** `const { [status]: word } = LABEL`: a read by a computed key, with no
 * default for a key the map does not hold. */
function destructuresUnanswered(
  node: ts.Node,
  checker: ts.TypeChecker,
  values: ResponseValues,
): node is ts.BindingElement {
  return (
    ts.isBindingElement(node) &&
    node.propertyName !== undefined &&
    ts.isComputedPropertyName(node.propertyName) &&
    node.initializer === undefined &&
    !admitsUndefined(checker.getTypeAtLocation(node.name)) &&
    values.holds(node.propertyName.expression)
  );
}

/** The findings in one module of a checked program. */
export function findingsIn(
  source: ts.SourceFile,
  rel: string,
  checker: ts.TypeChecker,
  values: ResponseValues,
): string[] {
  const out: string[] = [];
  const at = (node: ts.Node) =>
    `${rel}:${source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1}`;
  const visit = (node: ts.Node) => {
    if (
      readsUnanswered(node, checker, values) ||
      destructuresUnanswered(node, checker, values)
    ) {
      out.push(
        `${at(node)} \`${node.getText(source)}\` reads nothing for a value this build does not know`,
      );
    }
    if (ts.isSwitchStatement(node) && fallsOut(node, checker, values)) {
      out.push(
        `${at(node)} \`switch (${node.expression.getText(source)})\` covers every listed value and has no \`default\``,
      );
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return out;
}

/** A shipped module: product code, not a test, story, fixture, test kit or
 * the generated contract. */
export const shipped = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  /\.tsx?$/.test(file.rel) &&
  !file.rel.endsWith(".d.ts") &&
  !/\.(test|stories|testkit)\.tsx?$/.test(file.rel) &&
  !/fixture[^/]*$/.test(file.rel) &&
  !/(^|\/)src\/(api|testing)\//.test(file.rel);

const waiverFile = join(
  frontendRoot,
  "census",
  "response-enum-lookups-waivers.txt",
);

export type Waivers = Readonly<{
  counts: ReadonlyMap<string, number>;
  /** Each line that is not `<path> <count>`, as the finding to print. */
  problems: readonly string[];
}>;

const WAIVER_LINE = /^(\S+) ([1-9]\d*)$/;

/**
 * The count of findings each waived file still holds, read from lines of
 * `<path from frontend/> <count>`. A line in any other shape is a problem, so
 * a typo cannot turn the count checks off.
 */
export function readWaivers(text: string): Waivers {
  const counts = new Map<string, number>();
  const problems: string[] = [];
  text.split("\n").forEach((line, at) => {
    const trimmed = line.trim();
    if (trimmed === "" || trimmed.startsWith("#")) return;
    const parsed = WAIVER_LINE.exec(trimmed);
    const where = `census/response-enum-lookups-waivers.txt:${at + 1}`;
    if (!parsed?.[1] || !parsed[2]) {
      problems.push(
        `${where} is not \`<path> <count above zero>\`: ${trimmed}`,
      );
    } else if (counts.has(parsed[1])) {
      problems.push(`${where} lists ${parsed[1]} a second time`);
    } else {
      counts.set(parsed[1], Number(parsed[2]));
    }
  });
  return { counts, problems };
}

// The sum of the waiver list's counts. It may only fall: a fix lowers a count
// and this number in the same change.
export const WAIVED_LOOKUPS = 78;

/**
 * The findings left once the waivers are applied. A waived file must hold its
 * count. More is a new finding. Fewer means the count comes down, so it cannot
 * be spent again.
 */
export function unwaived(
  findings: readonly string[],
  waivers: ReadonlyMap<string, number>,
  read: ReadonlySet<string>,
  pinned = WAIVED_LOOKUPS,
): string[] {
  const byFile = new Map<string, string[]>();
  for (const finding of findings) {
    const path = finding.split(":")[0] ?? "";
    byFile.set(path, [...(byFile.get(path) ?? []), finding]);
  }
  const out: string[] = [];
  for (const [path, found] of byFile) {
    const waived = waivers.get(path) ?? 0;
    if (found.length > waived) out.push(...found);
    if (found.length < waived) {
      out.push(`${path}: lower its waiver count to ${found.length}`);
    }
  }
  for (const path of waivers.keys()) {
    if (read.has(path) && !byFile.has(path)) {
      out.push(`${path}: delete its waiver line, it has no finding left`);
    } else if (!existsSync(join(frontendRoot, path))) {
      out.push(`${path}: delete its waiver line, the file is gone`);
    }
  }
  const total = [...waivers.values()].reduce((sum, count) => sum + count, 0);
  if (total > pinned) {
    out.push(
      `census/response-enum-lookups-waivers.txt waives ${total} lookups and the pin is ${pinned}; the list only shrinks, so fix the lookup instead`,
    );
  }
  if (total < pinned) {
    out.push(
      `census/response-enum-lookups-waivers.txt waives ${total} lookups; lower WAIVED_LOOKUPS in response-enum-lookups.census.ts to ${total}`,
    );
  }
  return out;
}

export const rule: CensusRule = {
  name: "response-enum-lookups",
  corpus: (tree) => tree.files.filter(shipped),
  floors: (corpus) => {
    const { sets, lines, markers } = responseEnums(
      readFileSync(contractPath, "utf8"),
    );
    const out: string[] = [];
    if (corpus.length < 500) {
      out.push("fewer than 500 modules were read, so most lookups go unread");
    }
    if (markers < 500 || sets.length !== markers) {
      out.push(
        `crm.yaml holds ${markers} markers and ${sets.length} enums were read from them`,
      );
    }
    sets.forEach((set, at) => {
      if (set.size === 0) {
        out.push(`crm.yaml:${lines[at]} an enum was read with no values`);
      }
    });
    return out;
  },
  across: (corpus, tree) => {
    const { sets } = responseEnums(readFileSync(contractPath, "utf8"));
    const program = tree.checked();
    const checker = program.getTypeChecker();
    const values = new ResponseValues(checker, sets);
    const findings = corpus.flatMap((file) => {
      const source = program.getSourceFile(file.path);
      return source ? findingsIn(source, file.rel, checker, values) : [];
    });
    const waivers = readWaivers(readFileSync(waiverFile, "utf8"));
    return [
      ...waivers.problems,
      ...unwaived(
        findings,
        waivers.counts,
        new Set(corpus.map((file) => file.rel)),
      ),
    ];
  },
  advice:
    "a response enum is open, so a newer server can send a value this build " +
    "does not list, and these read nothing for it. Look the value up through " +
    "`labelFor` or `valueFor` (src/lib/labelfor.ts), type the map `Partial<…>` " +
    "and handle `undefined`, or give the switch a `default`",
};
