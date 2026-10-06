// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { filterView } from "./filters.testkit";
import {
  cutLibrary,
  groupOf,
  isFirstRun,
  type LibraryCut,
  type LibraryItem,
  libraryItems,
  nameOf,
  pillCounts,
  sortForReader,
  visibleGroups,
} from "./library";
import { liveList, shortlist } from "./lists.fixtures";
import type { List } from "./lists.queries";
import { isGroup } from "./segmentpredicate";

// The library as data: which rows it holds, where each sits, the order a
// reader reads them in, and what a cut leaves.

const BERLIN = { field: "city", op: "eq", value: "Berlin" };

const view = filterView("v1", "Berlin contacts", "contacts", {
  and: [BERLIN],
});
const mineList: List = {
  ...shortlist,
  id: "L-mine",
  name: "Dinner guests",
  sharing: "private",
  purpose: "Invited to the October dinner",
};
const archivedList: List = {
  ...liveList,
  id: "L-old",
  name: "Old campaign",
  archived_at: "2026-09-01T00:00:00Z",
};

const ALL: LibraryCut = { q: "", type: "all", archived: false };

/** The caption a row carries: a list's purpose, a view's filter as words. */
const captionOf = (item: LibraryItem) =>
  item.kind === "list" ? (item.list.purpose ?? "") : "City is Berlin";

describe("the merged library", () => {
  it("keeps only views whose filter this build can open", () => {
    const items = libraryItems(
      [
        view,
        filterView("v-old", "Stale", "contacts", {
          and: [{ field: "c", op: "like", value: "x" }],
        }),
        {
          ...filterView("v-list", "List state", "contacts", null),
          query: { list: {} },
        },
        filterView("v-act", "Calls", "activities", { and: [BERLIN] }),
        filterView("v-par", "Resellers", "partners", { and: [BERLIN] }),
        filterView("v-pro", "Builds", "projects", { and: [BERLIN] }),
      ],
      [],
    );
    expect(items.map(nameOf)).toEqual(["Berlin contacts"]);
  });

  it("wraps a stored single clause in a group, the shape every editor expects", () => {
    const [item] = libraryItems(
      [filterView("v-leaf", "One clause", "companies", BERLIN)],
      [],
    );
    expect(item.kind === "view" && isGroup(item.tree)).toBe(true);
  });

  it("files views and private lists under Only me, and team or workspace lists under Shared", () => {
    const items = libraryItems(
      [view],
      [mineList, liveList, { ...shortlist, sharing: "workspace" }],
    );
    expect(items.map(groupOf)).toEqual(["mine", "mine", "shared", "shared"]);
  });

  it("orders by name the way the reader reads an alphabet", () => {
    const named = (name: string): List => ({ ...shortlist, id: name, name });
    const items = libraryItems(
      [],
      [named("Zebra"), named("Ärzte"), named("Apfel")],
    );
    expect(sortForReader(items, "de").map(nameOf)).toEqual([
      "Apfel",
      "Ärzte",
      "Zebra",
    ]);
  });
});

describe("a cut", () => {
  const items = libraryItems([view], [mineList, liveList, archivedList]);

  it("narrows by record type", () => {
    expect(
      cutLibrary(items, { ...ALL, type: "companies" }, captionOf).map(nameOf),
    ).toEqual([mineList.name, liveList.name]);
    expect(
      cutLibrary(items, { ...ALL, type: "contacts" }, captionOf).map(nameOf),
    ).toEqual([view.name]);
  });

  it("searches the name or the caption, in any case", () => {
    expect(
      cutLibrary(items, { ...ALL, q: "DINNER" }, captionOf).map(nameOf),
    ).toEqual([mineList.name]);
    expect(
      cutLibrary(items, { ...ALL, q: "october" }, captionOf).map(nameOf),
    ).toEqual([mineList.name]);
    expect(
      cutLibrary(items, { ...ALL, q: "city is" }, captionOf).map(nameOf),
    ).toEqual([view.name]);
  });

  it("holds archived lists back until they are asked for", () => {
    expect(cutLibrary(items, ALL, captionOf).map(nameOf)).not.toContain(
      archivedList.name,
    );
    expect(
      cutLibrary(items, { ...ALL, archived: true }, captionOf).map(nameOf),
    ).toContain(archivedList.name);
  });
});

describe("the pill counts", () => {
  const items = libraryItems([view], [mineList, liveList, archivedList]);

  it("count each record type, archived lists only once asked for", () => {
    expect(pillCounts(items, false, false)).toEqual({
      all: 3,
      contacts: 1,
      companies: 2,
      deals: 0,
      leads: 0,
      projects: 0,
    });
    expect(pillCounts(items, true, false)?.companies).toBe(3);
  });

  it("say nothing while a read is short of the whole", () => {
    expect(pillCounts(items, false, true)).toBeUndefined();
  });
});

describe("the first run", () => {
  it("is a library holding no live view or list", () => {
    expect(isFirstRun([])).toBe(true);
    expect(isFirstRun(libraryItems([], [archivedList]))).toBe(true);
    expect(isFirstRun(libraryItems([view], []))).toBe(false);
  });
});

describe("the groups drawn", () => {
  const both = ["mine", "shared"] as const;
  const nothingOutstanding = () => false;
  const items = libraryItems([view], [liveList]);

  it("are both, with no cut, so an empty one can say what goes there", () => {
    const onlyMine = libraryItems([view], []);
    expect(visibleGroups(both, onlyMine, false, nothingOutstanding)).toEqual([
      "mine",
      "shared",
    ]);
  });

  it("are only those holding rows under a cut", () => {
    expect(visibleGroups(both, [items[0]], true, nothingOutstanding)).toEqual([
      "mine",
    ]);
    expect(visibleGroups(both, [], true, nothingOutstanding)).toEqual([]);
  });

  it("include one whose read is still out or failed", () => {
    expect(
      visibleGroups(both, [], true, (group) => group === "shared"),
    ).toEqual(["shared"]);
  });
});
