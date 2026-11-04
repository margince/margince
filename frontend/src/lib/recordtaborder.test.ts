// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join, relative } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { filesUnder, sourceFileAt } from "../../scripts/lib/source-tree";
import { CONTACT_TABS } from "./contacttab";
import { RECORD_TAB_ORDER } from "./recordtaborder";

// A record's tab list opens on Overview and satisfies `RecordTabId[]`, so an
// unknown id fails to compile. An untyped list of record tab ids alone is a
// record's strip that skipped the type, and the census takes it to fail it.
type TabList = Readonly<{ where: string; tabs: string[]; typed: boolean }>;

const SRC = join(__dirname, "..");

function isRecordTabType(node: ts.Expression): boolean {
  return (
    ts.isSatisfiesExpression(node) &&
    /\bRecordTabId\b/.test(node.type.getText())
  );
}

function arrayOf(node: ts.Expression): ts.ArrayLiteralExpression | undefined {
  if (ts.isArrayLiteralExpression(node)) return node;
  if (ts.isAsExpression(node) || ts.isSatisfiesExpression(node)) {
    return arrayOf(node.expression);
  }
  return undefined;
}

function slotOf(tab: string): number {
  return RECORD_TAB_ORDER.findIndex((ids) =>
    ids.some((id: string) => id === tab),
  );
}

function tabListOf(
  path: string,
  node: ts.VariableDeclaration,
): TabList | undefined {
  if (!node.initializer) return undefined;
  const tabs = arrayOf(node.initializer)?.elements.flatMap((each) =>
    ts.isStringLiteral(each) ? [each.text] : [],
  );
  if (tabs?.[0] !== "overview") return undefined;
  const typed = isRecordTabType(node.initializer);
  if (!typed && tabs.some((tab) => slotOf(tab) === -1)) return undefined;
  return {
    where: `${relative(SRC, path)} ${node.name.getText()}`,
    tabs,
    typed,
  };
}

function tabListsIn(path: string): TabList[] {
  const lists: TabList[] = [];
  const visit = (node: ts.Node): void => {
    const list = ts.isVariableDeclaration(node) && tabListOf(path, node);
    if (list) lists.push(list);
    ts.forEachChild(node, visit);
  };
  visit(sourceFileAt(path));
  return lists;
}

const lists = filesUnder(SRC)
  .filter((path) => !/\.(test|stories)\.tsx?$/.test(path))
  .flatMap(tabListsIn);

describe("the record tab order", () => {
  it("finds every record's tab list, the contact's among them", () => {
    expect(lists.length).toBeGreaterThanOrEqual(5);
    expect(lists.map((list) => list.tabs)).toContainEqual([...CONTACT_TABS]);
  });

  it.each(lists)("$where is typed as a record tab list", ({ typed }) => {
    expect(typed).toBe(true);
  });

  it.each(lists)("$where follows the shared order", ({ tabs }) => {
    const slots = tabs.map(slotOf);
    expect(slots).toEqual([...slots].sort((a, b) => a - b));
  });
});
