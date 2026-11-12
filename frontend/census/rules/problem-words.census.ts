// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A refusal's words reach a reader only through lib/problem.ts. The server's
// field message and problem detail are English fragments, so a screen that
// prints one shows English to a German or Vietnamese reader.
//
// A reader is found by type: a property read or destructured from a
// FieldProblem (lib/problembody.ts). A Problem or AiFieldError of the contract
// (api/schema.d.ts) counts too. Only the problem readers may read it.

import ts from "typescript";
import type { CensusFile, CensusRule, CensusTree } from "../tree";

// The words a screen must not print as they came from the server.
const SERVER_WORDS = new Set(["message", "detail", "title"]);

// The contract schemas whose words are the server's.
const CONTRACT_SCHEMAS = new Set(["Problem", "AiFieldError"]);

const OWNERS = /^src\/lib\/problem[a-z]*\.ts$/;

// Files that read the words without printing them, each with why.
export const WAIVED: ReadonlyMap<string, string> = new Map([
  [
    "src/features/analytics/analytics.assessment.tsx",
    "matches an older server's unknown-field sentence to tell a version gap apart; prints nothing",
  ],
  [
    "src/features/capture/capturevalue.ts",
    "matches the refusal's reason to pick catalog copy; prints only that copy or problemMessageOf",
  ],
  [
    "src/features/ai/ai-serving-editor.tsx",
    "an administrator's JSON editor: the validator's line names the path and value, which no catalog copy can",
  ],
]);

const isProductSource = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  /\.tsx?$/.test(file.rel) &&
  !/\.(test|stories|testrun)\.tsx?$/.test(file.rel) &&
  !file.rel.endsWith(".d.ts") &&
  !OWNERS.test(file.rel);

// A property is the server's words when its declaration sits in a field
// fault's type or in one of the contract schemas above.
function isServerWords(symbol: ts.Symbol | undefined): boolean {
  if (!symbol || !SERVER_WORDS.has(symbol.getName())) return false;
  return (symbol.declarations ?? []).some((declaration) => {
    const file = declaration.getSourceFile().fileName;
    if (file.endsWith("/src/lib/problembody.ts")) return true;
    if (!file.endsWith("/src/api/schema.d.ts")) return false;
    const schema = declaration.parent?.parent;
    return (
      schema !== undefined &&
      ts.isPropertySignature(schema) &&
      CONTRACT_SCHEMAS.has(schema.name.getText())
    );
  });
}

function readsIn(source: ts.SourceFile, checker: ts.TypeChecker): number[] {
  const lines: number[] = [];
  const report = (node: ts.Node) =>
    lines.push(source.getLineAndCharacterOfPosition(node.getStart()).line + 1);
  const visit = (node: ts.Node): void => {
    if (
      ts.isPropertyAccessExpression(node) &&
      SERVER_WORDS.has(node.name.text) &&
      isServerWords(checker.getSymbolAtLocation(node.name))
    ) {
      report(node);
    }
    if (ts.isBindingElement(node) && ts.isObjectBindingPattern(node.parent)) {
      const key = node.propertyName ?? node.name;
      // The name test comes first: asking the checker for a type is the costly step.
      if (ts.isIdentifier(key) && SERVER_WORDS.has(key.text)) {
        const owner = checker.getTypeAtLocation(node.parent);
        const unions = owner.isUnion() ? owner.types : [owner];
        if (unions.some((type) => isServerWords(type.getProperty(key.text)))) {
          report(node);
        }
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return lines;
}

export const rule: CensusRule = {
  name: "problem-words",
  corpus: (tree) => tree.files.filter(isProductSource),
  across: (corpus, tree: CensusTree) => {
    const program = tree.checked();
    const checker = program.getTypeChecker();
    const readers = new Set<string>();
    const findings = corpus.flatMap((file) => {
      const source = program.getSourceFile(file.path);
      if (!source) return [`${file.rel} is not in the type-checked program`];
      const lines = readsIn(source, checker);
      if (lines.length > 0) readers.add(file.rel);
      if (WAIVED.has(file.rel)) return [];
      return lines.map(
        (line) => `${file.rel}:${line} prints the server's words for a refusal`,
      );
    });
    const stale = [...WAIVED.keys()]
      .filter((rel) => corpus.some((file) => file.rel === rel))
      .filter((rel) => !readers.has(rel))
      .map((rel) => `${rel} is waived and reads no server words; remove it`);
    return [...findings, ...stale];
  },
  advice:
    "Read a refusal through lib/problem.ts: problemMessageOf for the callout, " +
    "problemFieldSentencesOf for the line under a field. Both give copy in the " +
    "reader's language, and the server's English only to an English reader.",
};
