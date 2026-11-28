// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The file lists' query keys are spelled once, in src/lib/attachmentkeys.ts,
// beside fileListKeys, which says which lists a written file lands in.
// A writer that spells a list's key by hand refreshes only the list it thought of.
// The others keep showing a file that moved or is gone.
// Tests count: a test that spells a key by hand checks a key production may no longer use.

import ts from "typescript";
import { mayHoldLiteral } from "../../scripts/lib/literal-text";
import type { CensusFile, CensusRule } from "../tree";

const OWNER = "src/lib/attachmentkeys.ts";

// A key whose first segment alone names a file list.
const LIST_ROOTS = new Set([
  "companyDocuments",
  "deal-documents",
  "deal-room-documents",
]);
// The attachments family, where the second segment names the list.
const ATTACHMENT_LISTS = new Set(["contact", "activity"]);

// A hand-spelled key names a list root or the attachments root.
const LIST_SPELLING = new RegExp([...LIST_ROOTS, "attachments"].join("|"));

function literalText(node: ts.Node | undefined): string | undefined {
  return node &&
    (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node))
    ? node.text
    : undefined;
}

function spellsListKey(array: ts.ArrayLiteralExpression): boolean {
  const [first, second] = array.elements;
  const root = literalText(first);
  if (root === undefined) return false;
  if (LIST_ROOTS.has(root)) return true;
  const list = literalText(second);
  return (
    root === "attachments" && list !== undefined && ATTACHMENT_LISTS.has(list)
  );
}

function handSpelledLines(source: ts.SourceFile): number[] {
  const lines: number[] = [];
  const visit = (node: ts.Node) => {
    if (ts.isArrayLiteralExpression(node) && spellsListKey(node)) {
      lines.push(
        source.getLineAndCharacterOfPosition(node.getStart()).line + 1,
      );
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return lines;
}

export const rule: CensusRule = {
  name: "document-list-keys",
  corpus: (tree) =>
    tree.files.filter((file) => file.area === "src" && file.source),
  floors: (corpus) => [
    ...(corpus.length > 100 ? [] : ["fewer than 100 files were read"]),
    ...(corpus.some((file) => file.rel === OWNER)
      ? []
      : [`${OWNER} was not read`]),
  ],
  visit: (file: CensusFile) =>
    file.rel === OWNER || !file.source || !mayHoldLiteral(file, LIST_SPELLING)
      ? []
      : handSpelledLines(file.source).map(
          (line) =>
            `${file.rel}:${line} spells a file list's query key by hand`,
        ),
  advice:
    "Read and refresh file lists through src/lib/attachmentkeys.ts: its key " +
    "builders for a reader, fileListKeys for a write that adds, moves or " +
    "removes a file.",
};
