// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { beforeEach, describe, expect, it } from "vitest";
import { viewerZone } from "../../format/timezone";
import { translate, translatePlural } from "../../i18n";
import type { VocabularyField } from "../../lib/filterdata";
import {
  decode,
  encode,
  type Group,
  isGroup,
  type Node,
  newGroup,
  newLeaf,
  resetIDsForTest,
} from "../../lib/segmentpredicate";
import {
  flipConnector,
  fromRow,
  groupDepth,
  rowOf,
  rowStart,
} from "./filterjoins";
import { filterSentence, type SentenceWords } from "./filtersentence";

// A row of conditions with a word between each pair, and the stored tree it
// means. "and" binds tighter than "or", so "A and B or C" is (A and B) or C.

beforeEach(resetIDsForTest);

const country = () => newLeaf("country", "eq", "DE");
const staff = () => newLeaf("employee_count", "gte", 50);
const city = () => newLeaf("city", "eq", "Berlin");

describe("a row read from a stored group", () => {
  it("reads an all-of group as one run of and", () => {
    const row = rowOf(newGroup("and", [country(), staff(), city()]));
    expect(row.joins).toEqual(["and", "and"]);
    expect(row.items.map((item) => item.id)).toEqual(["n1", "n2", "n3"]);
  });

  it("reads an any-of group as or between each", () => {
    expect(rowOf(newGroup("or", [country(), staff(), city()])).joins).toEqual([
      "or",
      "or",
    ]);
  });

  it("reads an all-of group inside an any-of group inline, as a run", () => {
    const tree = newGroup("or", [
      newGroup("and", [country(), staff()]),
      city(),
    ]);
    const row = rowOf(tree);
    expect(row.joins).toEqual(["and", "or"]);
    expect(row.items).toHaveLength(3);
    expect(rowStart(tree, 1)).toBe(2);
  });

  it("keeps an any-of group inside an all-of row as one condition", () => {
    const tree = newGroup("and", [
      country(),
      newGroup("or", [staff(), city()]),
    ]);
    expect(rowOf(tree).joins).toEqual(["and"]);
    expect(rowOf(tree).items).toHaveLength(2);
  });

  it("reads one condition and an empty group as rows with no word", () => {
    expect(rowOf(newGroup("and", [country()])).joins).toEqual([]);
    expect(rowOf(newGroup("or", [])).items).toEqual([]);
  });
});

describe("a row stored as a group", () => {
  it("stores a mixed row as or over and runs", () => {
    const [a, b, c] = [country(), staff(), city()];
    const stored = fromRow({ items: [a, b, c], joins: ["and", "or"] }, "root");
    expect(encode(stored)).toEqual({
      or: [{ and: [encode(a), encode(b)] }, encode(c)],
    });
  });

  it("stores a row of and as one all-of group, and of or as one any-of group", () => {
    const [a, b, c] = [country(), staff(), city()];
    expect(
      fromRow({ items: [a, b, c], joins: ["and", "and"] }, "root"),
    ).toEqual({ id: "root", join: "and", children: [a, b, c] });
    expect(fromRow({ items: [a, b, c], joins: ["or", "or"] }, "root")).toEqual({
      id: "root",
      join: "or",
      children: [a, b, c],
    });
  });

  it("keeps the join a row with no word had", () => {
    const a = country();
    expect(fromRow({ items: [a], joins: [] }, "root", "or")).toEqual({
      id: "root",
      join: "or",
      children: [a],
    });
    expect(fromRow({ items: [], joins: [] }, "root")).toEqual({
      id: "root",
      join: "and",
      children: [],
    });
  });

  it("gives back the same tree it was read from", () => {
    const trees: Group[] = [
      newGroup("and", [country(), staff(), city()]),
      newGroup("or", [country(), staff(), city()]),
      newGroup("or", [newGroup("and", [country(), staff()]), city()]),
      newGroup("or", [country(), newGroup("and", [staff(), city()])]),
      newGroup("and", [country(), newGroup("or", [staff(), city()])]),
      newGroup("or", [country()]),
      newGroup("or", [country(), staff(), newGroup("and", [city()])]),
      newGroup("and", []),
    ];
    for (const tree of trees) {
      expect(fromRow(rowOf(tree), tree.id, tree.join)).toEqual(tree);
    }
  });
});

describe("group ids through a flip", () => {
  function groupIDs(node: Node): string[] {
    return isGroup(node) ? [node.id, ...node.children.flatMap(groupIDs)] : [];
  }

  it("leaves a one-condition all-of group untouched when a word beside it flips", () => {
    const single = newGroup("and", [city()]);
    const tree = newGroup("or", [country(), staff(), single]);
    const flipped = flipConnector(tree, tree.id, 0).tree;
    expect(flipped.children).toHaveLength(2);
    expect(flipped.children[1]).toBe(single);
  });

  // A four-condition run, split at each of its three words.
  it.each([
    [0, [1, 3]],
    [1, [2, 2]],
    [2, [3, 1]],
  ])(
    "splits a run at word %i and keeps its id on the longest part",
    (at, sizes) => {
      const run = newGroup("and", [
        country(),
        staff(),
        city(),
        newLeaf("email", "exists", true),
      ]);
      const tree = newGroup("or", [run, newLeaf("full_name", "eq", "x")]);
      const flipped = flipConnector(tree, tree.id, at).tree;

      const parts = flipped.children.slice(0, 2);
      expect(
        parts.map((part) => (isGroup(part) ? part.children.length : 1)),
      ).toEqual(sizes);
      const longest = parts.find(
        (part) => isGroup(part) && part.children.length === Math.max(...sizes),
      );
      expect(longest?.id).toBe(run.id);
      const ids = groupIDs(flipped);
      expect(new Set(ids).size).toBe(ids.length);
    },
  );

  it("gives the id to the longer part when both parts stay runs", () => {
    const run = newGroup("and", [
      country(),
      staff(),
      city(),
      newLeaf("email", "exists", true),
      newLeaf("full_name", "eq", "x"),
    ]);
    const tree = newGroup("or", [run, newLeaf("owner_id", "eq", "u1")]);
    const [shorter, longer] = flipConnector(tree, tree.id, 1).tree.children;
    expect(longer?.id).toBe(run.id);
    expect(shorter?.id).not.toBe(run.id);
  });

  it("keeps one id when two runs merge into one", () => {
    const first = newGroup("and", [country(), staff()]);
    const second = newGroup("and", [city(), newLeaf("email", "exists", true)]);
    const tree = newGroup("or", [
      first,
      second,
      newLeaf("full_name", "eq", "x"),
    ]);
    const flipped = flipConnector(tree, tree.id, 1).tree;
    expect(flipped.children[0]?.id).toBe(first.id);
    const ids = groupIDs(flipped);
    expect(new Set(ids).size).toBe(ids.length);
  });
});

describe("flipping one word", () => {
  it("changes only the word pressed", () => {
    const tree = newGroup("and", [country(), staff(), city()]);
    const flipped = flipConnector(tree, tree.id, 1);
    expect(rowOf(flipped.tree).joins).toEqual(["and", "or"]);
    expect(encode(flipped.tree)).toEqual({
      or: [{ and: [encode(country()), encode(staff())] }, encode(city())],
    });
  });

  it("splits a run when its word turns to or, and merges runs back", () => {
    const tree = newGroup("or", [
      newGroup("and", [country(), staff()]),
      city(),
    ]);
    const split = flipConnector(tree, tree.id, 0);
    expect(rowOf(split.tree).joins).toEqual(["or", "or"]);
    const merged = flipConnector(split.tree, tree.id, 1);
    expect(rowOf(merged.tree).joins).toEqual(["or", "and"]);
    expect(encode(merged.tree)).toEqual({
      or: [encode(country()), { and: [encode(staff()), encode(city())] }],
    });
  });

  it("keeps the run's id when its first condition stays first", () => {
    const run = newGroup("and", [country(), staff(), city()]);
    const tree = newGroup("or", [run, newLeaf("email", "exists", true)]);
    const flipped = flipConnector(tree, tree.id, 1);
    expect(flipped.tree.children[0].id).toBe(run.id);
  });

  it("folds a group joined the way its parent is into the parent", () => {
    const inner = newGroup("or", [staff(), city()]);
    const tree = newGroup("and", [country(), inner]);
    const flipped = flipConnector(tree, inner.id, 0);
    expect(flipped.scope).toBe(tree.id);
    expect(flipped.tree).toEqual({
      ...tree,
      children: [tree.children[0], ...inner.children],
    });
  });

  it("changes nothing for an unknown group or a word out of range", () => {
    const tree = newGroup("and", [country(), staff()]);
    expect(flipConnector(tree, "missing", 0).tree).toBe(tree);
    expect(flipConnector(tree, tree.id, 1).tree).toBe(tree);
  });

  it("survives the wire: a saved mixed row loads with the words it was saved with", () => {
    const tree = newGroup("and", [country(), staff(), city()]);
    const saved = encode(flipConnector(tree, tree.id, 1).tree);
    const loaded = decode(JSON.parse(JSON.stringify(saved)));
    expect(loaded).not.toBeNull();
    expect(loaded && "children" in loaded && rowOf(loaded).joins).toEqual([
      "and",
      "or",
    ]);
  });

  it("counts the depth the engine bounds", () => {
    expect(groupDepth(country())).toBe(0);
    expect(groupDepth(newGroup("and", [country()]))).toBe(1);
    expect(
      groupDepth(newGroup("or", [newGroup("and", [country(), staff()])])),
    ).toBe(2);
  });
});

describe("the sentence a mixed row reads as", () => {
  const FIELDS: VocabularyField[] = [
    { name: "country", type: "text", operators: ["eq"], custom: false },
    {
      name: "employee_count",
      type: "number",
      operators: ["gte"],
      custom: false,
    },
    { name: "city", type: "text", operators: ["eq"], custom: false },
  ];
  const de: SentenceWords = {
    t: (key, params) => translate("de", key, params),
    plural: (base, count, params) => translatePlural("de", base, count, params),
    locale: "de",
    zone: viewerZone(),
  };

  it("brackets the and run, so the words read as the filter selects", () => {
    const tree = newGroup("and", [country(), staff(), city()]);
    const flipped = flipConnector(tree, tree.id, 1).tree;
    expect(filterSentence(flipped, FIELDS, de)).toBe(
      "(Ländercode ist DE und employee count ist mindestens 50) oder Stadt ist Berlin",
    );
  });
});
