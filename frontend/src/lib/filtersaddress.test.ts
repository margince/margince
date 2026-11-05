// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  type FiltersAddress,
  filtersAddressOf,
  LIBRARY_TYPES,
  libraryTypeOfList,
  OBJECT_TABS,
  opensAFocusedFiltersPage,
  tabOfListType,
  tabOfViewResource,
} from "./filtersaddress";

// Every address the Filters and views destination answers, and what each
// opens. The library is the answer to anything unrecognised, so a stale link
// lands where a reader can start rather than on a builder they never chose.

const ADDRESSES: readonly (readonly [
  Readonly<{ id?: string; id2?: string }>,
  FiltersAddress,
])[] = [
  [{}, { kind: "library" }],
  [{ id: "views" }, { kind: "library", anchor: "views" }],
  [{ id: "lists" }, { kind: "library", anchor: "lists" }],
  [{ id: "widgets" }, { kind: "library" }],
  [{ id: "list" }, { kind: "library" }],
  [{ id: "contacts" }, { kind: "new", tab: "contacts" }],
  [{ id: "companies" }, { kind: "new", tab: "companies" }],
  [{ id: "deals" }, { kind: "new", tab: "deals" }],
  [{ id: "leads" }, { kind: "new", tab: "leads" }],
  [
    { id: "contacts", id2: "v1" },
    { kind: "view", tab: "contacts", viewId: "v1" },
  ],
  [
    { id: "list", id2: "L1" },
    { kind: "listFilter", listId: "L1" },
  ],
];

describe("a Filters and views address", () => {
  it.each(ADDRESSES)("%j opens %j", (route, opens) => {
    expect(filtersAddressOf(route)).toEqual(opens);
  });

  it("heads itself exactly where a filter page or one list opens", () => {
    for (const [route, opens] of ADDRESSES) {
      expect(opensAFocusedFiltersPage({ screen: "filters", ...route })).toBe(
        opens.kind !== "library",
      );
    }
    expect(opensAFocusedFiltersPage({ screen: "lists", id: "L1" })).toBe(true);
    expect(opensAFocusedFiltersPage({ screen: "lists" })).toBe(false);
    expect(opensAFocusedFiltersPage({ screen: "contacts", id: "c1" })).toBe(
      false,
    );
  });
});

describe("the record type's words", () => {
  it("opens a list's filter on its own tab, and a project list's on none", () => {
    expect(tabOfListType("contact")).toBe("contacts");
    expect(tabOfListType("company")).toBe("companies");
    expect(tabOfListType("deal")).toBe("deals");
    expect(tabOfListType("lead")).toBe("leads");
    expect(tabOfListType("project")).toBeUndefined();
  });

  it("cuts a project list under projects, and every other list under its tab", () => {
    expect(libraryTypeOfList("project")).toBe("projects");
    expect(LIBRARY_TYPES).toEqual([...OBJECT_TABS, "projects"]);
  });

  it("opens a saved view only over a record type a filter is built on", () => {
    expect(tabOfViewResource("contacts")).toBe("contacts");
    expect(tabOfViewResource("leads")).toBe("leads");
    expect(tabOfViewResource("activities")).toBeUndefined();
    expect(tabOfViewResource("partners")).toBeUndefined();
    expect(tabOfViewResource("projects")).toBeUndefined();
  });
});
