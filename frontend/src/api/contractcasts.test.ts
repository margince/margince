// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { existsSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  filesMatching,
  parseSource,
  sourceFileAt,
} from "../../scripts/lib/source-tree";

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

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const sourceRoot = join(frontendRoot, "src");

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
    if (
      ts.isTypeAliasDeclaration(node) &&
      /\bcomponents\[/.test(node.type.getText(source))
    ) {
      out.add(node.name.text);
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return out;
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
  const text = target.getText(source);
  if (/\bcomponents\[\s*["']schemas["']\s*\]/.test(text)) {
    return true;
  }
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

type Finding = { where: string; line: number; says: string };

/** Every `x as unknown as T` in one module, with what it casts to. */
function castsIn(source: ts.SourceFile, where: string): Finding[] {
  const out: Finding[] = [];
  const aliases = contractAliases(source, join(frontendRoot, where));
  const lines = source.getFullText().split("\n");
  const visit = (node: ts.Node) => {
    if (
      ts.isAsExpression(node) &&
      ts.isAsExpression(node.expression) &&
      node.expression.type.kind === ts.SyntaxKind.UnknownKeyword &&
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

describe("a fixture is the shape the server sends", () => {
  const modules = filesMatching(sourceRoot, /\.tsx?$/);

  // A CENSUS THAT READ NOTHING reports the same silence as one that found
  // nothing wrong, and this one is armed at zero — so an empty corpus would
  // look exactly like a clean tree.
  it("reads the tree it audits", () => {
    expect(
      modules.length,
      "no modules were found, so no cast would be read at all",
    ).toBeGreaterThan(500);
  });

  // A WHOLE-TREE CENSUS IN ONE CASE, so the budget is the tree's and not a
  // single assertion's. vitest's per-test default is ten seconds; this reads
  // every module the bundler would, and on a loaded CI runner under coverage
  // the same work that takes a second here took just over ten and failed on
  // the clock rather than on a finding. A census that reports its own runner
  // as a defect teaches the next reader to ignore it, so the budget is stated
  // and generous. Splitting the loop across cases to fit would buy the same
  // seconds by hiding where they go.
  it("finds no contract type cast through unknown", () => {
    const found: string[] = [];
    for (const path of modules) {
      const text = readFileSync(path, "utf8");
      // THE ONLY THING THIS SKIPS IS A FILE THAT CANNOT HOLD THE CONSTRUCT.
      // `x as unknown as T` contains the token `unknown` whatever trivia sits
      // between its parts, so a file without that word has no such cast to
      // find — it is a necessary condition and not a heuristic, which is what
      // separates it from a skip-list. It buys the census the headroom it
      // needs: 619 of this tree's 2096 modules carry the word, and parsing the
      // other 1477 is work whose answer is known before it starts.
      if (!text.includes("unknown")) {
        continue;
      }
      const where = relative(frontendRoot, path).replaceAll("\\", "/");
      for (const one of castsIn(parseSource(path, text), where)) {
        found.push(`${one.where}:${one.line} ${one.says}`);
      }
    }

    expect(
      found.sort(),
      "these remove the type check rather than widening it: a fixture missing a " +
        "required field compiles, and the case asserts over a shape the server " +
        "never sends. Give the fixture the fields the type requires, or say why " +
        "the shape is wrong on purpose with `contract:cast <reason>`",
    ).toEqual([]);
  }, 30_000);
});

// THE DETECTOR, asked about every shape it has to read.
//
// The census above can only report what this recognises, so a reader that
// quietly stopped seeing one shape would take the whole gate to PASS with
// nothing failing. Two of the four cases below are the ways it has already been
// wrong: a type imported rather than declared, and a waiver written as a
// paragraph rather than a line.
describe("what counts as a contract type cast", () => {
  const found = (code: string) =>
    castsIn(parseSource(probePath, code), "src/screens/probe.ts").map(
      (one) => one.says,
    );

  // A real module, so the import arm resolves against a file that exists. This
  // one declares `Worklist` as a contract alias and re-exports it, which is the
  // shape the screens use.
  const probePath = join(sourceRoot, "screens", "probe.ts");
  const aliasing = 'import type { Worklist } from "./worklist.queries";\n';

  it("finds a cast to a type the module declares", () => {
    expect(
      found(
        'type Deal = components["schemas"]["Deal"];\nconst d = {} as unknown as Deal;',
      ),
    ).toEqual(["`as unknown as Deal`"]);
  });

  it("finds a cast to a contract type the module imported", () => {
    expect(found(`${aliasing}const w = {} as unknown as Worklist;`)).toEqual([
      "`as unknown as Worklist`",
    ]);
  });

  it("finds a cast through a modifier on a contract type", () => {
    expect(
      found(`${aliasing}const w = {} as unknown as Partial<Worklist>;`),
    ).toEqual(["`as unknown as Partial<Worklist>`"]);
    expect(
      found('const o = {} as unknown as components["schemas"]["Offer"];'),
    ).toEqual(['`as unknown as components["schemas"]["Offer"]`']);
  });

  // The DOM, the router and the shape of a mock are a different question: a
  // cast to one of those says nothing about whether a fixture matches the wire.
  it("passes over a cast to something that is not the contract", () => {
    expect(found("const e = {} as unknown as Event;")).toEqual([]);
    expect(found("const m = fetch as unknown as { calls: number };")).toEqual(
      [],
    );
  });

  it("takes a reason from anywhere in the comment block above", () => {
    expect(
      found(
        `${aliasing}// contract:cast \`{}\` is not a page, on purpose.\n// The case is about what the reader survives.\nconst w = {} as unknown as Worklist;`,
      ),
    ).toEqual([]);
  });

  // A marker with no reason says a check was skipped and not why.
  it("refuses a marker with no reason", () => {
    expect(
      found(
        `${aliasing}// contract:cast\nconst w = {} as unknown as Worklist;`,
      ),
    ).toEqual([
      "`contract:cast` with no reason beside `as unknown as Worklist`",
    ]);
  });
});
