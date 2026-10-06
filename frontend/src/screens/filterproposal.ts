// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A model's proposal, landed in the builder as ordinary editable rows that
// carry a mark until the reader makes them their own. The mark lives on the
// leaf and nowhere else, so `encode` drops it with the id: a saved filter
// never knows which of its clauses a model proposed.
//
// Every operation hands back each node it leaves alone as the same object:
// `landProposal` tells the rows it added from the reader's own by identity, so
// a re-minted node of theirs would be counted as proposed.

import {
  type Group,
  isGroup,
  type Leaf,
  type Node,
  rootGroup,
} from "./segmentpredicate";

/** A proposal on screen: what was asked, the tree before it, and what it added. */
export type Proposal = Readonly<{
  text: string;
  /** The tree before the FIRST proposal still on screen, which Undo restores. */
  before: Group;
  /** The root's children the proposal added, edited since or not. */
  nodeIds: readonly string[];
  /** Whether the reader had conditions of their own beside it. */
  hadOwn: boolean;
}>;

/**
 * The proposed tree added to the one on screen: its clauses join the root when
 * both combine the same way, and arrive as one group of their own otherwise.
 * A stored single clause decodes to a bare leaf, which becomes the root's
 * first child.
 */
export function addProposal(current: Node, proposed: Node): Group {
  const root = rootGroup(current);
  const added =
    isGroup(proposed) && proposed.join === root.join
      ? proposed.children
      : [proposed];
  return { ...root, children: [...root.children, ...added] };
}

/** Every leaf in the group marked as proposed. */
export function markProposed(group: Group): Group {
  return { ...group, children: group.children.map(marked) };
}

function marked(node: Node): Node {
  return isGroup(node) ? markProposed(node) : { ...node, proposed: true };
}

export function proposedCount(tree: Node): number {
  if (!isGroup(tree)) {
    return tree.proposed ? 1 : 0;
  }
  return tree.children.reduce((sum, child) => sum + proposedCount(child), 0);
}

/** The leaf as the reader's own: the same clause, unmarked. */
export function ownLeaf(leaf: Leaf): Leaf {
  if (!leaf.proposed) {
    return leaf;
  }
  const { proposed: _accepted, ...own } = leaf;
  return own;
}

/**
 * The tree after the reader changed how one group joins. Every leaf in that
 * group is theirs from then on, as an edited leaf is, so a second proposal
 * cannot take away a group they reworked. An unknown id changes nothing.
 */
export function ownGroup(tree: Group, groupId: string): Group {
  if (tree.id === groupId) {
    return acceptProposals(tree);
  }
  return withChildren(
    tree,
    tree.children.map((child) =>
      isGroup(child) ? ownGroup(child, groupId) : child,
    ),
  );
}

/** The tree with every remaining mark dropped: Keep all. */
export function acceptProposals(tree: Group): Group {
  return withChildren(
    tree,
    tree.children.map((child) =>
      isGroup(child) ? acceptProposals(child) : ownLeaf(child),
    ),
  );
}

/**
 * The tree without the proposed leaves nobody touched. A group that only held
 * them goes with them; a group the reader left empty themselves stays, because
 * removing it would be an edit they did not make.
 */
export function withoutProposed(tree: Group): Group {
  return withChildren(tree, unmarkedChildren(tree.children));
}

function unmarkedChildren(children: readonly Node[]): Node[] {
  return children.flatMap((child): Node[] => {
    if (!isGroup(child)) {
      return child.proposed ? [] : [child];
    }
    if (child.children.length === 0) {
      return [child];
    }
    const kept = withoutProposed(child);
    return kept.children.length === 0 ? [] : [kept];
  });
}

/**
 * A proposal landed on the tree on screen when it ARRIVES.
 *
 * A second proposal while the first still has untouched rows replaces only
 * those rows: what the reader edited is theirs and stays. Undo still returns to
 * the tree before the first, so `before` is carried while any mark remains.
 */
export function landProposal(
  current: Group,
  proposed: Group,
  text: string,
  previous: Proposal | null,
): Readonly<{ tree: Group; proposal: Proposal }> {
  const live = previous !== null && proposedCount(current) > 0;
  const own = live ? withoutProposed(current) : current;
  const before = live ? previous.before : current;
  const incoming = markProposed(proposed);
  const tree =
    own.children.length === 0
      ? { ...own, join: incoming.join, children: incoming.children }
      : addProposal(own, incoming);
  const nodeIds = tree.children
    .filter((child) => !own.children.includes(child))
    .map((child) => child.id);
  return {
    tree,
    proposal: { text, before, nodeIds, hadOwn: own.children.length > 0 },
  };
}

/**
 * Replace my conditions: only the proposal's own top-level nodes, edited or
 * not, under the root's id. One group is unwrapped into the root so its join
 * survives; several nodes keep the join they sit under, so an OR proposal is
 * never read back as an AND.
 */
export function replaceWithProposal(tree: Group, proposal: Proposal): Group {
  const kept = tree.children.filter((child) =>
    proposal.nodeIds.includes(child.id),
  );
  const [only] = kept;
  if (kept.length === 1 && only !== undefined && isGroup(only)) {
    return { id: tree.id, join: only.join, children: only.children };
  }
  return { id: tree.id, join: tree.join, children: kept };
}

/** The group with these children, or the group itself when none changed. */
function withChildren(group: Group, children: readonly Node[]): Group {
  const same =
    children.length === group.children.length &&
    children.every((child, index) => child === group.children[index]);
  return same ? group : { ...group, children };
}
