// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A record's tab list satisfies `RecordTabId[]`, so an unknown id fails to
// compile. An untyped list that opens on Overview and holds another record tab
// id is a strip that skipped the type. The census takes it, so it fails.

import ts from "typescript";
import { CONTACT_TABS } from "../../src/lib/contacttab";
import { RECORD_TAB_ORDER } from "../../src/lib/recordtaborder";
import type { CensusFile, CensusRule } from "../tree";

type TabList = Readonly<{ where: string; tabs: string[]; typed: boolean }>;

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
  rel: string,
  node: ts.VariableDeclaration,
): TabList | undefined {
  if (!node.initializer) return undefined;
  const tabs = arrayOf(node.initializer)?.elements.flatMap((each) =>
    ts.isStringLiteral(each) ? [each.text] : [],
  );
  if (!tabs) return undefined;
  const typed = isRecordTabType(node.initializer);
  const looksLikeStrip =
    tabs[0] === "overview" && tabs.slice(1).some((tab) => slotOf(tab) !== -1);
  if (!typed && !looksLikeStrip) return undefined;
  return { where: `${rel} ${node.name.getText()}`, tabs, typed };
}

function tabListsIn(file: CensusFile): TabList[] {
  const lists: TabList[] = [];
  const visit = (node: ts.Node): void => {
    const list = ts.isVariableDeclaration(node) && tabListOf(file.rel, node);
    if (list) lists.push(list);
    ts.forEachChild(node, visit);
  };
  if (file.source) visit(file.source);
  return lists;
}

/** What is wrong with one tab list, one sentence per fault. */
export function faultsOf({ where, tabs, typed }: TabList): string[] {
  const slots = tabs.map(slotOf);
  return [
    ...(typed ? [] : [`${where} is not typed as a record tab list`]),
    ...(tabs[0] === "overview" ? [] : [`${where} does not open on Overview`]),
    ...(slots.every((slot, at) => at === 0 || (slots[at - 1] ?? 0) <= slot)
      ? []
      : [`${where} does not follow RECORD_TAB_ORDER`]),
  ];
}

const inCorpus = (file: CensusFile) =>
  file.area === "src" &&
  /\.tsx?$/.test(file.path) &&
  !/\.(test|stories)\.tsx?$/.test(file.path);

export const rule: CensusRule = {
  name: "recordtaborder",
  corpus: (tree) => tree.files.filter(inCorpus),
  floors: (corpus) => {
    const lists = corpus.flatMap(tabListsIn);
    const contact = JSON.stringify([...CONTACT_TABS]);
    return [
      ...(lists.length >= 5
        ? []
        : ["fewer than 5 record tab lists were found"]),
      ...(lists.some((list) => JSON.stringify(list.tabs) === contact)
        ? []
        : ["the contact's tab list was not found"]),
    ];
  },
  visit: (file) => tabListsIn(file).flatMap(faultsOf),
  advice:
    "A record's tab list satisfies RecordTabId[], opens on Overview and " +
    "follows RECORD_TAB_ORDER in src/lib/recordtaborder.ts.",
};
