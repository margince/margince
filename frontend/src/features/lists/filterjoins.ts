// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The words between a group's conditions, each one its own switch. The stored
// tree keeps one join per group. So a mixed row is held the way a reader reads
// it, with "and" binding tighter. Each run of "and" is an all-of group inside
// an any-of group: "A and B or C" is stored as `{or: [{and: [A, B]}, C]}`.

import {
  type Group,
  isGroup,
  type Node,
  newGroup,
  replaceNode,
} from "../../lib/segmentpredicate";

export type Join = Group["join"];

/**
 * A group read as one line: its conditions, and the word after each but the
 * last. `runs` names the stored all-of group each "and" run came from, by its
 * first condition, so a regrouped row keeps the ids React keys on.
 */
export type Row = Readonly<{
  items: readonly Node[];
  joins: readonly Join[];
  runs?: Readonly<Record<string, string>>;
}>;

/**
 * Whether a child is an "and" run of its parent's row. An all-of group with
 * nothing in it stays a group of its own: it has no conditions to read inline.
 */
export function isRun(child: Node, parent: Group): boolean {
  return runItems(child, parent) !== null;
}

function runItems(child: Node, parent: Group): readonly Node[] | null {
  return parent.join === "or" &&
    isGroup(child) &&
    child.join === "and" &&
    child.children.length > 0
    ? child.children
    : null;
}

/** How many of the row's conditions come before the child at `index`. */
export function rowStart(group: Group, index: number): number {
  return group.children
    .slice(0, index)
    .reduce((sum, child) => sum + (runItems(child, group)?.length ?? 1), 0);
}

export function rowOf(group: Group): Row {
  const items: Node[] = [];
  const joins: Join[] = [];
  const runs: Record<string, string> = {};
  group.children.forEach((child, index) => {
    if (index > 0) {
      joins.push(group.join);
    }
    const run = runItems(child, group);
    if (run === null) {
      items.push(child);
      return;
    }
    run.forEach((item, at) => {
      if (at === 0) {
        runs[item.id] = child.id;
      }
      if (at > 0) {
        joins.push("and");
      }
      items.push(item);
    });
  });
  return { items, joins, runs };
}

/**
 * The row as a stored group with this id. A row with no word in it says
 * nothing about its join, so it keeps `fallback`, the join it had.
 */
export function fromRow(row: Row, id: string, fallback: Join = "and"): Group {
  const runs = splitRuns(row);
  if (runs.length <= 1) {
    const join = row.joins.length === 0 ? fallback : "and";
    return { id, join, children: runs[0] ?? [] };
  }
  return {
    id,
    join: "or",
    children: runs.flatMap((run): Node[] => {
      const [first] = run;
      if (first === undefined || run.length === 1) {
        return run;
      }
      const kept = row.runs?.[first.id];
      return [
        kept === undefined
          ? newGroup("and", run)
          : { id: kept, join: "and", children: run },
      ];
    }),
  };
}

function splitRuns(row: Row): Node[][] {
  const runs: Node[][] = [];
  let run: Node[] = [];
  row.items.forEach((item, index) => {
    if (index > 0 && row.joins[index - 1] === "or") {
      runs.push(run);
      run = [];
    }
    run.push(item);
  });
  if (run.length > 0) {
    runs.push(run);
  }
  return runs;
}

/**
 * The tree with one word of one row flipped, and the id of the group whose
 * conditions it regrouped. A regrouped group joined the way its parent is
 * dissolves into the parent. "A and (B and C)" says nothing a flat row does
 * not. An unknown id or index changes nothing.
 */
export function flipConnector(
  tree: Group,
  ownerID: string,
  index: number,
): Readonly<{ tree: Group; scope: string }> {
  const found = locate(tree, ownerID, null);
  const row = found === null ? null : rowOf(found.group);
  if (
    found === null ||
    row === null ||
    index < 0 ||
    index >= row.joins.length
  ) {
    return { tree, scope: ownerID };
  }
  const joins = row.joins.map((join, at) =>
    at === index ? (join === "and" ? "or" : "and") : join,
  );
  const regrouped = fromRow({ ...row, joins }, ownerID, found.group.join);
  const { parent } = found;
  if (parent !== null && parent.join === regrouped.join) {
    const merged: Group = {
      ...parent,
      children: parent.children.flatMap((child) =>
        child.id === ownerID ? regrouped.children : [child],
      ),
    };
    return { tree: swap(tree, merged), scope: parent.id };
  }
  return { tree: swap(tree, regrouped), scope: ownerID };
}

function locate(
  node: Node,
  id: string,
  parent: Group | null,
): Readonly<{ group: Group; parent: Group | null }> | null {
  if (!isGroup(node)) {
    return null;
  }
  if (node.id === id) {
    return { group: node, parent };
  }
  for (const child of node.children) {
    const found = locate(child, id, node);
    if (found !== null) {
      return found;
    }
  }
  return null;
}

// The replacement is a group, so the root stays a group whichever node it is.
function swap(tree: Group, group: Group): Group {
  const next = replaceNode(tree, group.id, () => group);
  return next !== null && isGroup(next) ? next : tree;
}

/** How many groups the deepest condition sits inside: the engine's depth. */
export function groupDepth(node: Node): number {
  return isGroup(node) ? 1 + Math.max(0, ...node.children.map(groupDepth)) : 0;
}
