// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { type Translator, translate } from "../i18n";
import {
  type FiltersAddress,
  filtersAddressOf,
  isObjectTab,
  LIBRARY_TYPES,
  libraryTypeOfList,
  listEntityTypeOf,
  OBJECT_TABS,
  opensAFocusedFiltersPage,
  resourceOf,
  tabOfListType,
  tabOfViewResource,
  tabWords,
  viewOf,
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

  it("lets an extension's kind be a tab that names itself", () => {
    const room = "ext:hotel:room";
    expect(isObjectTab(room)).toBe(true);
    expect(isObjectTab("contacts")).toBe(true);
    expect(isObjectTab("ext:Hotel:room")).toBe(false);
    expect(isObjectTab("widgets")).toBe(false);
    expect(isObjectTab(undefined)).toBe(false);
    expect(resourceOf(room)).toBe(room);
    expect(viewOf(room)).toBe(room);
    expect(resourceOf("companies")).toBe("company");
    expect(viewOf("companies")).toBe("companies");
    expect(listEntityTypeOf(room)).toBeUndefined();
    expect(listEntityTypeOf("deals")).toBe("deal");
  });

  it("opens an extension's list or view on its own tab", () => {
    const room = "ext:hotel:room";
    expect(tabOfListType(room)).toBe(room);
    expect(libraryTypeOfList(room)).toBe(room);
    expect(tabOfViewResource(room)).toBe(room);
    expect(tabOfViewResource("ext:nope")).toBeUndefined();
  });

  it("addresses an extension's filter page and one of its views", () => {
    expect(filtersAddressOf({ id: "ext:hotel:room" })).toEqual({
      kind: "new",
      tab: "ext:hotel:room",
    });
    expect(filtersAddressOf({ id: "ext:hotel:room", id2: "v1" })).toEqual({
      kind: "view",
      tab: "ext:hotel:room",
      viewId: "v1",
    });
  });

  it("words an extension's tab with the directory's name", () => {
    const t: Translator = (key, params) => translate("en", key, params);
    const words = tabWords("ext:hotel:room", t, [
      { name: "ext:hotel:room", display_name: "Room" },
    ]);
    expect(words.tab).toBe("Room");
    expect(words.newFilter).toBe("New Room filter");
    expect(words.unit).toBe("Room records");
    expect(words.matchParams).toEqual({ kind: "Room" });
    expect(tabWords("contacts", t, []).newFilter).toBe("New contact filter");
  });

  it("opens a saved view only over a record type a filter is built on", () => {
    expect(tabOfViewResource("contacts")).toBe("contacts");
    expect(tabOfViewResource("leads")).toBe("leads");
    expect(tabOfViewResource("activities")).toBeUndefined();
    expect(tabOfViewResource("partners")).toBeUndefined();
    expect(tabOfViewResource("projects")).toBeUndefined();
  });
});
