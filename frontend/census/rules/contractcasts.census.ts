// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { existsSync } from "node:fs";
import { dirname, join } from "node:path";
import ts from "typescript";
import { sourceFileAt } from "../../scripts/lib/source-tree";
import { type CensusRule, frontendRoot } from "../tree";

// `as unknown as <a contract type>` DOES NOT WIDEN THE TYPE CHECK, IT REMOVES
// IT.
//
// A fixture missing a required field compiles, and the case asserts over a
// shape the server never sends. Nothing fails, which is what makes the class
// silent: #5534 found an enum value the server does not send
// (`waiting_customer` for `customer_waiting`), two required fields missing from
// every row fixture and six from three page fixtures — with all thirty-four
// cases passing throughout. The sweep this gate was armed after found the same
// enum value again in a second file, a `phase` the enum does not hold, deals
// with no pipeline and no stage, an offer with no `captured_by`, and contacts
// the wire could not carry. Every case passed before and after.
//
// A story counts. A story is the fixture the UAT lane renders, so one over a
// wrong shape renders a screen the server cannot produce.
//
// The waiver is `contract:cast <reason>` in a comment on the line or the line
// above, which is how `{}` as a payload stays sayable: it is not valid, on
// purpose, and a cast is the only way to spell that. A marker with no reason is
// itself a finding.

/**
 * The type aliases in a module that name a contract shape — its own, and the
 * ones it imports.
 *
 * Read per file rather than through the checker: every one of them is written
 * the same way — `type Deal = components["schemas"]["Deal"]` — and a cast to a
 * type this cannot recognise is a cast to something that is not the contract,
 * which is a different question (a DOM `Event`, the shape of a mock).
 *
 * IMPORTS ARE FOLLOWED, one module deep. A screen's query module re-exports the
 * contract type its screen reads — `import type { Worklist } from
 * "./worklist.queries"` — and a reader that stopped at the file in front of it
 * called that cast something other than the contract, which is the direction
 * this gate must not fail in.
 */
function contractAliases(source: ts.SourceFile, path: string): Set<string> {
  const out = declaredAliases(source);
  for (const statement of source.statements) {
    if (
      !ts.isImportDeclaration(statement) ||
      !ts.isStringLiteral(statement.moduleSpecifier) ||
      !statement.moduleSpecifier.text.startsWith(".")
    ) {
      continue;
    }
    const bindings = statement.importClause?.namedBindings;
    if (!bindings || !ts.isNamedImports(bindings)) {
      continue;
    }
    const exported = aliasesExportedBy(
      join(dirname(path), statement.moduleSpecifier.text),
    );
    for (const element of bindings.elements) {
      if (exported.has((element.propertyName ?? element.name).text)) {
        out.add(element.name.text);
      }
    }
  }
  return out;
}

function declaredAliases(source: ts.SourceFile): Set<string> {
  const out = new Set<string>();
  const visit = (node: ts.Node) => {
    if (ts.isTypeAliasDeclaration(node) && indexesTheContract(node.type)) {
      out.add(node.name.text);
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return out;
}

/**
 * Whether a type indexes into the generated contract — `components["schemas"][…]`
 * anywhere inside it, however it is written.
 *
 * READ FROM THE TREE, not from the text. `components ["schemas"]["Deal"]` is the
 * same type with one space in it, and a pattern requiring the bracket to sit
 * against the identifier called it something other than the contract. That is
 * under-recognition, which is the one direction this gate must not fail in: a
 * cast it does not recognise is a cast it reports as clean.
 */
function indexesTheContract(node: ts.Node): boolean {
  if (ts.isIndexedAccessTypeNode(node) && namesComponents(node.objectType)) {
    return true;
  }
  return ts.forEachChild(node, indexesTheContract) ?? false;
}

/** The base of an indexed access, following the chain down to its root name. */
function namesComponents(node: ts.TypeNode): boolean {
  if (ts.isIndexedAccessTypeNode(node)) {
    return namesComponents(node.objectType);
  }
  return (
    ts.isTypeReferenceNode(node) &&
    ts.isIdentifier(node.typeName) &&
    node.typeName.text === "components"
  );
}

/** The contract aliases one module declares, read once and remembered. */
const exportedAliases = new Map<string, Set<string>>();

function aliasesExportedBy(base: string): Set<string> {
  const known = exportedAliases.get(base);
  if (known) {
    return known;
  }
  let found = new Set<string>();
  for (const suffix of [".ts", ".tsx", "/index.ts", "/index.tsx"]) {
    const path = base + suffix;
    if (existsSync(path)) {
      found = declaredAliases(sourceFileAt(path));
      break;
    }
  }
  exportedAliases.set(base, found);
  return found;
}

/** Whether a cast's target names a contract shape. */
function namesAContractShape(
  target: ts.TypeNode,
  aliases: Set<string>,
  source: ts.SourceFile,
): boolean {
  if (indexesTheContract(target)) {
    return true;
  }
  const text = target.getText(source);
  // `Partial<Company360>` and `NonNullable<…>` are the contract shape with a
  // modifier on it, so the name inside is what decides.
  for (const name of text.matchAll(/\b([A-Z][A-Za-z0-9_]*)\b/g)) {
    if (aliases.has(name[1])) {
      return true;
    }
  }
  return false;
}

type Waiver = "none" | "reasoned" | "bare";

/**
 * The waiver sitting with one cast: on its own line, or anywhere in the comment
 * block directly above it.
 *
 * THE WHOLE BLOCK, because a reason worth writing is usually a paragraph — the
 * three this tree has all open with what `{}` is and close with why no typed
 * fixture can say it. A waiver readable only on the line immediately above
 * would push the marker away from the sentence that explains it.
 *
 * A reason is the words after the marker. A marker with none says a check was
 * skipped and not why, which is the note the next reader cannot act on.
 */
function waiverAt(lines: string[], line: number): Waiver {
  for (let at = line; at >= 0; at--) {
    const text = lines[at] ?? "";
    const marker = /contract:cast\b(.*)$/.exec(text);
    if (marker) {
      return marker[1].trim().length > 0 ? "reasoned" : "bare";
    }
    // Past the cast's own line, only a comment block carries a waiver: the
    // first line of code above it ends the search.
    if (at < line && !/^\s*(\/\/|\*|\/\*)/.test(text)) {
      return "none";
    }
  }
  return "none";
}

/**
 * Whether the expression being cast is itself `… as unknown`, through however
 * many parentheses.
 *
 * `(value as unknown) as Deal` is the same cast with brackets round the middle
 * of it, and a reader that demanded the inner `as` be the immediate child read
 * it as an ordinary widening and passed over it.
 */
function bridgesThroughUnknown(expression: ts.Expression): boolean {
  const inner = ts.isParenthesizedExpression(expression)
    ? bare(expression.expression)
    : expression;
  return (
    ts.isAsExpression(inner) && inner.type.kind === ts.SyntaxKind.UnknownKeyword
  );
}

function bare(expression: ts.Expression): ts.Expression {
  return ts.isParenthesizedExpression(expression)
    ? bare(expression.expression)
    : expression;
}

// `as`, then whitespace or comments, then `unknown`: the bridge however it is
// spaced or annotated. A line comment ends at any of TypeScript's four line
// terminators, not only at LF.
const AS_UNKNOWN =
  /\bas(?:\s|\/\*[\s\S]*?\*\/|\/\/[^\n\r\u2028\u2029]*(?:[\n\r\u2028\u2029]|$))+unknown\b/;
export const castsToUnknown = (text: string) => AS_UNKNOWN.test(text);

type Finding = { where: string; line: number; says: string };

/** Every `x as unknown as T` in one module, with what it casts to. */
export function castsIn(source: ts.SourceFile, where: string): Finding[] {
  const out: Finding[] = [];
  const aliases = contractAliases(source, join(frontendRoot, where));
  // TypeScript's own line starts, so the lines are the ones its line numbers
  // count, at every terminator it reads.
  const text = source.getFullText();
  const starts = source.getLineStarts();
  const lines = starts.map((start, at) =>
    text
      .slice(start, starts[at + 1])
      .replace(/(?:\r\n|[\n\r\u2028\u2029])$/, ""),
  );
  const visit = (node: ts.Node) => {
    if (
      ts.isAsExpression(node) &&
      bridgesThroughUnknown(node.expression) &&
      namesAContractShape(node.type, aliases, source)
    ) {
      const line =
        source.getLineAndCharacterOfPosition(node.type.getStart(source)).line +
        1;
      const waiver = waiverAt(lines, line - 1);
      if (waiver !== "reasoned") {
        out.push({
          where,
          line,
          says:
            waiver === "bare"
              ? `\`contract:cast\` with no reason beside \`as unknown as ${node.type.getText(source)}\``
              : `\`as unknown as ${node.type.getText(source)}\``,
        });
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return out;
}

export const rule: CensusRule = {
  name: "contractcasts",
  // A declaration file holds no expression, so no `as` cast, and the
  // generated contract types are megabytes no run needs to read.
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        file.area === "src" &&
        /\.tsx?$/.test(file.path) &&
        !file.path.endsWith(".d.ts"),
    ),
  // A CENSUS THAT READ NOTHING reports the same silence as one that found
  // nothing wrong, and this one is armed at zero.
  floors: (corpus) =>
    corpus.length > 500
      ? []
      : ["fewer than 500 modules were read, so most casts would go unread"],
  // The only thing this skips is a file that cannot hold the construct.
  // `x as unknown as T` holds the tokens `as` and `unknown` with nothing
  // but trivia between them, so a file without that pair has no such cast
  // to find. It is a necessary condition and not a heuristic.
  visit: (file) =>
    file.source && castsToUnknown(file.text)
      ? castsIn(file.source, file.rel).map(
          (one) => `${one.where}:${one.line} ${one.says}`,
        )
      : [],
  advice:
    "these remove the type check rather than widening it: a fixture missing a " +
    "required field compiles, and the case asserts over a shape the server " +
    "never sends. Give the fixture the fields the type requires, or say why " +
    "the shape is wrong on purpose with `contract:cast <reason>`",
};
