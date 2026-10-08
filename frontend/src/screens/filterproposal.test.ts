// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { beforeEach, describe, expect, it } from "vitest";
import {
  acceptProposals,
  addProposal,
  landProposal,
  markProposed,
  ownGroup,
  ownLeaf,
  type Proposal,
  proposedCount,
  replaceWithProposal,
  withoutProposed,
} from "./filterproposal";
import {
  decode,
  encode,
  type Group,
  isGroup,
  type Leaf,
  type Node,
  newGroup,
  newLeaf,
  replaceNode,
  resetIDsForTest,
  rootGroup,
} from "./segmentpredicate";

// A proposal lands as rows the reader can edit, each marked until they make it
// theirs. These pin where the rows land, what a second proposal and Undo give
// back, and that a mark never reaches the wire.

beforeEach(() => {
  resetIDsForTest();
});

function decoded(stored: unknown): Group {
  const tree = decode(stored);
  if (tree === null) {
    throw new Error("the fixture tree did not decode");
  }
  return rootGroup(tree);
}

function shapeOf(node: Node): unknown {
  return JSON.parse(
    JSON.stringify(node, (key, value) => (key === "id" ? undefined : value)),
  );
}

function leafAt(tree: Group, index: number): Leaf {
  const child = tree.children[index];
  if (child === undefined || isGroup(child)) {
    throw new Error(`child ${index} is not a leaf`);
  }
  return child;
}

/** The reader's edit: a changed value makes the row theirs, as the builder does. */
function editValue(tree: Group, id: string, value: string): Group {
  return rootGroup(
    replaceNode(tree, id, (found) =>
      isGroup(found) ? found : { ...ownLeaf(found), value },
    ) ?? tree,
  );
}

const cityIs = (city: string) => ({ field: "city", op: "eq", value: city });

describe("marking a proposal", () => {
  it("marks every leaf at any depth and counts them", () => {
    const deep = decoded({
      and: [
        cityIs("Berlin"),
        { or: [cityIs("Wien"), { and: [{ or: [cityIs("Graz")] }] }] },
      ],
    });
    const marked = markProposed(deep);
    expect(proposedCount(deep)).toBe(0);
    expect(proposedCount(marked)).toBe(3);
    const graz = { field: "city", op: "eq", value: "Graz", proposed: true };
    expect(marked).toMatchObject({
      join: "and",
      children: [
        { field: "city", value: "Berlin", proposed: true },
        {
          join: "or",
          children: [
            { field: "city", value: "Wien", proposed: true },
            { join: "and", children: [{ join: "or", children: [graz] }] },
          ],
        },
      ],
    });
  });

  it("accepts every mark and keeps the nodes it did not change", () => {
    const own = newGroup("or", [newLeaf("city", "eq", "Bonn")]);
    const tree = newGroup("and", [
      own,
      ...markProposed(decoded({ and: [cityIs("Wien")] })).children,
    ]);
    const kept = acceptProposals(tree);
    expect(proposedCount(kept)).toBe(0);
    expect(kept.children[0]).toBe(own);
    const untouched = newGroup("and", [own]);
    expect(acceptProposals(untouched)).toBe(untouched);
    const leaf = newLeaf("city", "eq", "Bonn");
    expect(ownLeaf(leaf)).toBe(leaf);
    expect(ownLeaf({ ...leaf, proposed: true })).toEqual(leaf);
  });

  it("drops untouched proposals and a group they emptied, but not a group the reader left empty", () => {
    const readersEmpty = newGroup("or");
    const proposedGroup = markProposed(
      decoded({ or: [cityIs("Wien"), cityIs("Graz")] }),
    );
    const own = newLeaf("city", "eq", "Bonn");
    const tree = newGroup("and", [own, readersEmpty, proposedGroup]);
    const left = withoutProposed(tree);
    expect(left.children).toEqual([own, readersEmpty]);
    expect(left.children[1]).toBe(readersEmpty);
    const unmarked = newGroup("and", [own, readersEmpty]);
    expect(withoutProposed(unmarked)).toBe(unmarked);
  });
});

describe("landing a proposal", () => {
  it("fills an empty root under its own id, in the proposal's join", () => {
    const empty = newGroup("and");
    const and = landProposal(
      empty,
      decoded({ and: [cityIs("Wien"), cityIs("Graz")] }),
      "Wien and Graz",
      null,
    );
    expect(and.tree.id).toBe(empty.id);
    expect(shapeOf(and.tree)).toEqual({
      join: "and",
      children: [
        { ...cityIs("Wien"), proposed: true },
        { ...cityIs("Graz"), proposed: true },
      ],
    });
    expect(and.proposal).toMatchObject({
      text: "Wien and Graz",
      before: empty,
      hadOwn: false,
    });
    expect(and.proposal.nodeIds).toEqual(
      and.tree.children.map((child) => child.id),
    );

    const or = landProposal(
      empty,
      decoded({ or: [cityIs("Wien"), cityIs("Graz")] }),
      "either",
      null,
    );
    expect(or.tree).toMatchObject({ id: empty.id, join: "or" });
  });

  it("splices into a root joined the same way and arrives as one group otherwise", () => {
    const andRoot = newGroup("and", [newLeaf("city", "eq", "Bonn")]);
    const spliced = landProposal(
      andRoot,
      decoded({ and: [cityIs("Wien")] }),
      "Wien",
      null,
    );
    expect(shapeOf(spliced.tree)).toEqual({
      join: "and",
      children: [cityIs("Bonn"), { ...cityIs("Wien"), proposed: true }],
    });
    expect(spliced.tree.children[0]).toBe(andRoot.children[0]);
    expect(spliced.proposal.hadOwn).toBe(true);

    const orProposal = landProposal(
      andRoot,
      decoded({ or: [cityIs("Wien"), cityIs("Graz")] }),
      "either",
      null,
    );
    expect(shapeOf(orProposal.tree)).toMatchObject({
      join: "and",
      children: [cityIs("Bonn"), { join: "or" }],
    });
    expect(orProposal.proposal.nodeIds).toEqual([
      orProposal.tree.children[1]?.id,
    ]);

    const orRoot = newGroup("or", [
      newLeaf("city", "eq", "Bonn"),
      newLeaf("city", "eq", "Kiel"),
    ]);
    const andProposal = landProposal(
      orRoot,
      decoded({
        and: [cityIs("Wien"), { field: "email", op: "exists", value: true }],
      }),
      "Wien with mail",
      null,
    );
    expect(shapeOf(andProposal.tree)).toMatchObject({
      join: "or",
      children: [cityIs("Bonn"), cityIs("Kiel"), { join: "and" }],
    });
  });

  it("replaces only the untouched rows of a live proposal and keeps the first before", () => {
    const start = newGroup("and", [newLeaf("city", "eq", "Bonn")]);
    const first = landProposal(
      start,
      decoded({ and: [cityIs("Wien"), cityIs("Graz")] }),
      "first",
      null,
    );
    const editedId = leafAt(first.tree, 1).id;
    const edited = editValue(first.tree, editedId, "Linz");
    const second = landProposal(
      edited,
      decoded({ and: [cityIs("Salzburg")] }),
      "second",
      first.proposal,
    );
    expect(shapeOf(second.tree)).toEqual({
      join: "and",
      children: [
        cityIs("Bonn"),
        cityIs("Linz"),
        { ...cityIs("Salzburg"), proposed: true },
      ],
    });
    expect(second.proposal.before).toBe(start);
    expect(second.proposal.text).toBe("second");

    const accepted = acceptProposals(second.tree);
    const third = landProposal(
      accepted,
      decoded({ and: [cityIs("Kiel")] }),
      "third",
      second.proposal,
    );
    expect(third.proposal.before).toBe(accepted);
  });
});

describe("replacing my conditions", () => {
  function landedOn(
    current: Group,
    stored: unknown,
  ): Readonly<{ tree: Group; proposal: Proposal }> {
    return landProposal(current, decoded(stored), "asked", null);
  }

  it("keeps only the proposal's rows, edited or not", () => {
    const mine = newGroup("and", [newLeaf("city", "eq", "Bonn")]);
    const { tree, proposal } = landedOn(mine, {
      and: [cityIs("Wien"), cityIs("Graz")],
    });
    const edited = editValue(tree, leafAt(tree, 2).id, "Linz");
    const replaced = replaceWithProposal(edited, proposal);
    expect(replaced.id).toBe(mine.id);
    expect(shapeOf(replaced)).toEqual({
      join: "and",
      children: [{ ...cityIs("Wien"), proposed: true }, cityIs("Linz")],
    });
  });

  it("unwraps one kept group into the root, keeping its join", () => {
    const mine = newGroup("and", [newLeaf("city", "eq", "Bonn")]);
    const { tree, proposal } = landedOn(mine, {
      or: [cityIs("Wien"), cityIs("Graz")],
    });
    const replaced = replaceWithProposal(tree, proposal);
    expect(replaced.id).toBe(mine.id);
    expect(shapeOf(replaced)).toMatchObject({
      join: "or",
      children: [cityIs("Wien"), cityIs("Graz")],
    });
  });

  it("never reads an OR proposal's rows back as an AND", () => {
    const mine = newGroup("or", [newLeaf("city", "eq", "Bonn")]);
    const { tree, proposal } = landedOn(mine, {
      or: [cityIs("Wien"), cityIs("Graz")],
    });
    expect(replaceWithProposal(tree, proposal).join).toBe("or");
  });
});

describe("the wire never carries a mark", () => {
  it("encodes every operation's result exactly as its accepted twin", () => {
    const mine = newGroup("and", [
      newLeaf("city", "eq", "Bonn"),
      newGroup("or"),
    ]);
    const landed = landProposal(
      mine,
      decoded({ or: [cityIs("Wien"), { and: [cityIs("Graz")] }] }),
      "asked",
      null,
    );
    const results: Group[] = [
      markProposed(
        decoded({ and: [cityIs("Wien"), { or: [cityIs("Graz")] }] }),
      ),
      landed.tree,
      withoutProposed(landed.tree),
      replaceWithProposal(landed.tree, landed.proposal),
      acceptProposals(landed.tree),
      addProposal(mine, landed.tree),
    ];
    for (const result of results) {
      const wire = encode(result);
      expect(wire).toEqual(encode(acceptProposals(result)));
      expect(JSON.stringify(wire)).not.toContain("proposed");
    }
  });
});

it("adds a proposal joined the same way into the root, and any other as one group", () => {
  const current = newGroup("and", [newLeaf("full_name", "eq", "Ann")]);
  const sameJoin = decode({
    and: [{ field: "city", op: "eq", value: "Berlin" }],
  });
  const otherJoin = decode({
    or: [
      { field: "city", op: "eq", value: "Berlin" },
      { field: "city", op: "eq", value: "Wien" },
    ],
  });
  if (sameJoin === null || otherJoin === null) {
    throw new Error("fixture trees did not decode");
  }
  expect(shapeOf(addProposal(current, sameJoin))).toMatchObject({
    join: "and",
    children: [{ field: "full_name" }, { field: "city" }],
  });
  const grouped = shapeOf(addProposal(current, otherJoin));
  expect(grouped).toMatchObject({
    join: "and",
    children: [{ field: "full_name" }, { join: "or" }],
  });
});

it("keeps a stored single clause, which decodes to a bare leaf, beside what it adds", () => {
  const stored = decode({ field: "full_name", op: "eq", value: "Ann" });
  const proposed = decode({ and: [cityIs("Berlin")] });
  if (stored === null || proposed === null) {
    throw new Error("fixture trees did not decode");
  }
  const added = addProposal(stored, proposed);
  expect(shapeOf(added)).toEqual({
    join: "and",
    children: [
      { field: "full_name", op: "eq", value: "Ann" },
      cityIs("Berlin"),
    ],
  });
  expect(added.children[0]).toBe(stored);
});

describe("a group whose join the reader changed", () => {
  /** The reader's edit: the group's join flipped, which makes it theirs. */
  function toggleJoin(tree: Group, groupId: string): Group {
    const toggled = replaceNode(tree, groupId, (found) =>
      isGroup(found)
        ? { ...found, join: found.join === "and" ? "or" : "and" }
        : found,
    );
    return ownGroup(rootGroup(toggled ?? tree), groupId);
  }

  function groupAt(tree: Group, index: number): Group {
    const child = tree.children[index];
    if (child === undefined || !isGroup(child)) {
      throw new Error(`child ${index} is not a group`);
    }
    return child;
  }

  it("is the reader's own at every depth, and nothing else changes", () => {
    const mine = newGroup("and", [newLeaf("city", "eq", "Bonn")]);
    const { tree } = landProposal(
      mine,
      decoded({
        and: [
          cityIs("Kiel"),
          { or: [cityIs("Wien"), { and: [cityIs("Graz")] }] },
        ],
      }),
      "asked",
      null,
    );
    const group = groupAt(tree, 2);
    const owned = ownGroup(tree, group.id);
    expect(proposedCount(tree)).toBe(3);
    expect(proposedCount(groupAt(owned, 2))).toBe(0);
    expect(proposedCount(owned)).toBe(1);
    expect(owned.children[0]).toBe(tree.children[0]);
    expect(owned.children[1]).toBe(tree.children[1]);
    expect(ownGroup(tree, "no-such-group")).toBe(tree);
  });

  it("survives a second proposal, which replaces only the untouched rows", () => {
    const mine = newGroup("and", [newLeaf("city", "eq", "Bonn")]);
    const first = landProposal(
      mine,
      decoded({ or: [cityIs("Wien"), cityIs("Graz")] }),
      "first",
      null,
    );
    const reworked = toggleJoin(first.tree, groupAt(first.tree, 1).id);
    const second = landProposal(
      reworked,
      decoded({ and: [cityIs("Salzburg")] }),
      "second",
      first.proposal,
    );
    expect(shapeOf(second.tree)).toEqual({
      join: "and",
      children: [
        cityIs("Bonn"),
        { join: "and", children: [cityIs("Wien"), cityIs("Graz")] },
        { ...cityIs("Salzburg"), proposed: true },
      ],
    });
    // Nothing proposed was left untouched, so Undo stops at the reworked tree.
    expect(second.proposal.before).toBe(reworked);
  });
});
