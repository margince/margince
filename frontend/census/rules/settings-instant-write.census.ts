// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A pick on a settings page is a draft the page's save bar commits; only a
// `Switch` writes when turned. The rule finds an `onChange` on any other
// element that reaches a mutation. It looks in the handler's own body, in a
// function of the same file, and in a prop the same file hands a mutation to.

import ts from "typescript";
import type { CensusFile, CensusRule } from "../tree";

const WRITES = new Set(["mutate", "mutateAsync"]);

type Scope = Readonly<{
  /** Same-file functions by name, for `onChange={pick}` or a call to one. */
  functions: ReadonlyMap<string, ts.Node>;
  /** Each component's handed props by component and prop name. */
  handed: ReadonlyMap<string, readonly ts.Expression[]>;
}>;

const isFunction = (
  node: ts.Node | undefined,
): node is ts.ArrowFunction | ts.FunctionExpression =>
  node !== undefined &&
  (ts.isArrowFunction(node) || ts.isFunctionExpression(node));

function scopeOf(source: ts.SourceFile): Scope {
  const functions = new Map<string, ts.Node>();
  const handed = new Map<string, ts.Expression[]>();
  const visit = (node: ts.Node) => {
    if (ts.isFunctionDeclaration(node) && node.name && node.body) {
      functions.set(node.name.text, node.body);
    }
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      isFunction(node.initializer)
    ) {
      functions.set(node.name.text, node.initializer.body);
    }
    if (ts.isJsxAttribute(node) && ts.isIdentifier(node.name)) {
      const value = node.initializer;
      const element = node.parent.parent;
      if (
        value &&
        ts.isJsxExpression(value) &&
        value.expression &&
        (ts.isJsxOpeningElement(element) || ts.isJsxSelfClosingElement(element))
      ) {
        const key = `${element.tagName.getText(source)}.${node.name.text}`;
        handed.set(key, [...(handed.get(key) ?? []), value.expression]);
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return { functions, handed };
}

// The component an element sits in, and the names its first parameter binds.
function enclosingProps(node: ts.Node): { name: string; props: Set<string> } {
  for (let at = node.parent; at; at = at.parent) {
    const fn =
      ts.isFunctionDeclaration(at) && at.name
        ? { name: at.name.text, params: at.parameters }
        : isFunction(at) &&
            ts.isVariableDeclaration(at.parent) &&
            ts.isIdentifier(at.parent.name)
          ? { name: at.parent.name.text, params: at.parameters }
          : undefined;
    if (fn && /^[A-Z]/.test(fn.name)) {
      const first = fn.params[0]?.name;
      const props = new Set<string>();
      if (first && ts.isObjectBindingPattern(first)) {
        for (const element of first.elements) {
          if (ts.isIdentifier(element.name)) props.add(element.name.text);
        }
      }
      return { name: fn.name, props };
    }
  }
  return { name: "", props: new Set() };
}

function reachesWrite(start: ts.Node, scope: Scope, seen: Set<ts.Node>) {
  if (seen.has(start)) return false;
  seen.add(start);
  const { name, props } = enclosingProps(start);
  const follow = (called: string): readonly ts.Node[] => {
    const local = scope.functions.get(called);
    if (local) return [local];
    return props.has(called)
      ? (scope.handed.get(`${name}.${called}`) ?? [])
      : [];
  };
  let found = false;
  const visit = (node: ts.Node) => {
    if (found) return;
    if (
      ts.isPropertyAccessExpression(node) &&
      WRITES.has(node.name.text) &&
      ts.isCallExpression(node.parent) &&
      node.parent.expression === node
    ) {
      found = true;
      return;
    }
    const named = ts.isIdentifier(node) ? follow(node.text) : [];
    if (named.some((next) => reachesWrite(next, scope, seen))) {
      found = true;
      return;
    }
    ts.forEachChild(node, visit);
  };
  visit(start);
  return found;
}

export type InstantWrite = Readonly<{ line: number; tag: string }>;

/** Every `onChange` in `source` that writes on change, Switches included. */
export function instantWritesIn(source: ts.SourceFile): InstantWrite[] {
  const scope = scopeOf(source);
  const writes: InstantWrite[] = [];
  const visit = (node: ts.Node) => {
    if (
      ts.isJsxAttribute(node) &&
      node.name.getText(source) === "onChange" &&
      node.initializer &&
      reachesWrite(node.initializer, scope, new Set())
    ) {
      const element = node.parent.parent as ts.JsxOpeningLikeElement;
      writes.push({
        line:
          source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1,
        tag: element.tagName.getText(source),
      });
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return writes;
}

// A control that writes on change, each with why it is not a page's draft.
// The count must match both ways, so a new one fails and a fixed one frees it.
const WAIVED: ReadonlyMap<
  string,
  Readonly<{ count: number; why: string }>
> = new Map([
  [
    "src/features/settings/lossreasons.tsx",
    {
      count: 1,
      why: "A review queue: each pick decides one note's row, not a setting.",
    },
  ],
  [
    "src/features/settings/roles-detail.tsx",
    {
      count: 1,
      why: "The role drawer writes each grant and scope as picked; its head says a refusal.",
    },
  ],
  [
    "src/features/settings/team-dialogs.tsx",
    {
      count: 1,
      why: "A team's member list: picking a person adds them, as each row's verb removes.",
    },
  ],
]);

const judged = (file: CensusFile) =>
  file.area === "src" &&
  file.source !== undefined &&
  file.rel.startsWith("src/features/settings/") &&
  file.rel.endsWith(".tsx") &&
  !/\.(test|stories)\.tsx$/.test(file.rel);

const notSwitches = (file: CensusFile) =>
  file.source
    ? instantWritesIn(file.source).filter((write) => write.tag !== "Switch")
    : [];

export const rule: CensusRule = {
  name: "settings-instant-write",
  corpus: (tree) => tree.files.filter(judged),
  // The Switches that write on change are the walk's proof that it sees one.
  floors: (corpus) => [
    ...(corpus.some(
      (file) =>
        file.source !== undefined &&
        instantWritesIn(file.source).some((write) => write.tag === "Switch"),
    )
      ? []
      : ["the walk found no Switch that writes on change"]),
    ...[...WAIVED.keys()]
      .filter((rel) => !corpus.some((file) => file.rel === rel))
      .map((rel) => `${rel} is waived but no longer exists`),
  ],
  visit: (file) => {
    const writes = notSwitches(file);
    const waived = WAIVED.get(file.rel);
    if (waived) {
      return writes.length === waived.count
        ? []
        : [
            `${file.rel}: waived for ${waived.count} and holds ${writes.length}; change its waiver to match`,
          ];
    }
    return writes.map(
      (write) => `${file.rel}:${write.line}: <${write.tag}> writes on change`,
    );
  },
  advice:
    "Make the pick a draft the page's PageSaveBar commits (useSaveDraft in " +
    "src/design-system/savepage.tsx). Only a Switch writes when turned.",
};
