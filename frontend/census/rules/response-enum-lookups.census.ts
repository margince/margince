// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import ts from "typescript";
import { appCompilerOptions } from "../../scripts/lib/source-tree";
import {
  type CensusFile,
  type CensusRule,
  type CensusTree,
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

/**
 * Whether a union is one the frontend names itself, as `type Locale = "en" |
 * "de" | "vi"`. Such a vocabulary is the app's own, and a value reaches it
 * from outside only through a guard like `isLocale`. The generated contract
 * names no union: schema.d.ts writes each enum inline.
 */
function namedByTheFrontend(type: ts.Type): boolean {
  const declared = type.aliasSymbol?.declarations?.[0]?.getSourceFile();
  return (
    declared !== undefined && !/[\\/]src[\\/]api[\\/]/.test(declared.fileName)
  );
}

/** Whether a type is a response enum, or a narrowing of one. */
function isResponseEnum(
  type: ts.Type,
  sets: readonly ReadonlySet<string>[],
): boolean {
  if (namedByTheFrontend(type)) return false;
  const literals = literalsOf(type);
  if (!literals || literals.length < 2) return false;
  return sets.some((set) => literals.every((value) => set.has(value)));
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
 * reader answer the missing case. */
const admitsUndefined = (type: ts.Type) =>
  (type.isUnion() ? type.types : [type]).some(
    (part) => part.flags & (ts.TypeFlags.Undefined | ts.TypeFlags.Any),
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
  sets: readonly ReadonlySet<string>[],
): boolean {
  const clauses = node.caseBlock.clauses;
  if (clauses.some(ts.isDefaultClause)) return false;
  const type = checker.getTypeAtLocation(node.expression);
  const literals = literalsOf(type);
  if (!literals || !isResponseEnum(type, sets)) return false;
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

/** Whether the code around a read answers a missing value: `M[v] ?? x` or
 * `M[v]?.x`. */
const answeredInPlace = (node: ts.Node): boolean => {
  const parent = node.parent;
  if (
    ts.isBinaryExpression(parent) &&
    parent.left === node &&
    parent.operatorToken.kind === ts.SyntaxKind.QuestionQuestionToken
  ) {
    return true;
  }
  return (
    (ts.isPropertyAccessExpression(parent) ||
      ts.isElementAccessExpression(parent) ||
      ts.isCallExpression(parent)) &&
    parent.questionDotToken !== undefined
  );
};

/** Whether `node` reads a map by a response enum value and nothing answers a
 * value the map does not hold. */
export function readsUnanswered(
  node: ts.Node,
  checker: ts.TypeChecker,
  sets: readonly ReadonlySet<string>[],
): node is ts.ElementAccessExpression {
  return (
    ts.isElementAccessExpression(node) &&
    !ts.isLiteralExpression(node.argumentExpression) &&
    !isWriteTarget(node) &&
    !answeredInPlace(node) &&
    isResponseEnum(checker.getTypeAtLocation(node.argumentExpression), sets) &&
    !admitsUndefined(checker.getTypeAtLocation(node))
  );
}

/** The findings in one module of a checked program. */
export function findingsIn(
  source: ts.SourceFile,
  rel: string,
  checker: ts.TypeChecker,
  sets: readonly ReadonlySet<string>[],
): string[] {
  const out: string[] = [];
  const at = (node: ts.Node) =>
    `${rel}:${source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1}`;
  const visit = (node: ts.Node) => {
    if (readsUnanswered(node, checker, sets)) {
      out.push(
        `${at(node)} \`${node.getText(source)}\` reads nothing for a value this build does not know`,
      );
    }
    if (ts.isSwitchStatement(node) && fallsOut(node, checker, sets)) {
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

/** A checked program over the corpus, built from the trees the census parsed. */
export function programOf(
  corpus: readonly CensusFile[],
  tree: CensusTree,
): ts.Program {
  const options = {
    ...appCompilerOptions(frontendRoot),
    noEmit: true,
    incremental: false,
  };
  const host = ts.createCompilerHost(options, true);
  const read = host.getSourceFile.bind(host);
  host.getSourceFile = (name, version, onError, create) =>
    tree.at(name)?.source ?? read(name, version, onError, create);
  return ts.createProgram(
    corpus.map((file) => file.path),
    options,
    host,
  );
}

const waiverFile = join(
  frontendRoot,
  "census",
  "response-enum-lookups-waivers.txt",
);

/**
 * The count of findings each waived file still holds, read from lines of
 * `<path from frontend/> <count>`.
 */
export function readWaivers(text: string): Map<string, number> {
  const out = new Map<string, number>();
  for (const line of text.split("\n")) {
    const trimmed = line.trim();
    if (trimmed === "" || trimmed.startsWith("#")) continue;
    const [path = "", count = ""] = trimmed.split(/\s+/);
    out.set(path, Number(count));
  }
  return out;
}

// The sum of the waiver list's counts. It may only fall: a fix lowers a count
// and this number in the same change.
export const WAIVED_LOOKUPS = 117;

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
    const program = programOf(corpus, tree);
    const checker = program.getTypeChecker();
    const findings = corpus.flatMap((file) => {
      const source = program.getSourceFile(file.path);
      return source ? findingsIn(source, file.rel, checker, sets) : [];
    });
    return unwaived(
      findings,
      readWaivers(readFileSync(waiverFile, "utf8")),
      new Set(corpus.map((file) => file.rel)),
    );
  },
  advice:
    "a response enum is open, so a newer server can send a value this build " +
    "does not list, and these read nothing for it. Look the value up through " +
    "`labelFor` or `valueFor` (src/lib/labelfor.ts), type the map `Partial<…>` " +
    "and handle `undefined`, or give the switch a `default`",
};
