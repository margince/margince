// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { listOpening } from "../../components/listquery.options";
import { listQueryFromParams } from "../../components/listqueryparams";
import { matchesView } from "../../components/listqueryviews";
import { leadsOpeningFilters } from "../../lib/leads.address";
import {
  COMPANIES_INITIAL_SORT,
  CONTACTS_INITIAL_SORT,
  LEADS_INITIAL_SORT,
  PROJECTS_INITIAL_SORT,
} from "../../lib/listopenings";
import type { List } from "../../lib/lists.queries";
import type { SavedView } from "../../lib/savedviews.queries";
import { isGroup } from "../../lib/segmentpredicate";
import { parseParams } from "../../lib/urlstate";
import {
  liveList,
  OTHER_OWNER_ID,
  READER_ID,
  shortlist,
} from "../../testing/lists.fixtures";
import { filterView } from "./filters.testkit";
import {
  cutLibrary,
  groupOf,
  hrefOf,
  isFirstRun,
  type LibraryCut,
  type LibraryItem,
  libraryItems,
  listsLanding,
  listViewHref,
  nameOf,
  ownedByReader,
  pillCounts,
  sortForReader,
  typeOf,
  visibleGroups,
  withFound,
} from "./library";

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
  owner_id: READER_ID,
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

/** A view saved on a list screen: its dials, and no filter tree. */
function listView(
  id: string,
  name: string,
  resource: string,
  list: unknown,
): SavedView {
  return { ...filterView(id, name, resource, null), query: { list } };
}

const byName = listView("v-name", "By name", "contacts", {
  q: "",
  sort: "name",
  includeArchived: false,
  filters: {},
});

describe("the merged library", () => {
  it("keeps the views whose filter or list state this build can open", () => {
    const items = libraryItems(
      [
        view,
        filterView("v-old", "Stale", "contacts", {
          and: [{ field: "c", op: "like", value: "x" }],
        }),
        listView("v-list", "List state", "contacts", {}),
        listView("v-junk", "Junk", "contacts", "sort=name"),
        filterView("v-act", "Calls", "activities", { and: [BERLIN] }),
        listView("v-act-list", "Call list", "activities", {}),
        filterView("v-par", "Resellers", "partners", { and: [BERLIN] }),
        listView("v-par-list", "Reseller list", "partners", {}),
        filterView("v-pro", "Builds", "projects", { and: [BERLIN] }),
      ],
      [],
    );
    expect(items.map(nameOf)).toEqual(["Berlin contacts", "List state"]);
    expect(items.map((item) => item.kind)).toEqual(["view", "listView"]);
  });

  it("files a view saved on the Projects list under the projects pill", () => {
    const items = libraryItems(
      [listView("v-pro-list", "Stalled builds", "projects", { sort: "name" })],
      [],
    );
    expect(items.map(typeOf)).toEqual(["projects"]);
    expect(pillCounts(items, false, false)?.projects).toBe(1);
    expect(groupOf(items[0], READER_ID)).toBe("mine");
  });

  it("opens a view holding both a filter and list state wherever it is pressed: in the builder", () => {
    const both: SavedView = {
      ...view,
      query: { ...view.query, list: { sort: "name" } },
    };
    const items = libraryItems([both], []);
    expect(items).toHaveLength(1);
    expect(items[0]?.kind).toBe("view");
  });

  it("wraps a stored single clause in a group, the shape every editor expects", () => {
    const [item] = libraryItems(
      [filterView("v-leaf", "One clause", "companies", BERLIN)],
      [],
    );
    expect(item.kind === "view" && isGroup(item.tree)).toBe(true);
  });

  it("files views and the reader's own lists as theirs, whoever can find them, and every other list as shared", () => {
    const theirs = { ...liveList, owner_id: OTHER_OWNER_ID };
    const filed = (list: List) =>
      groupOf(libraryItems([], [list])[0], READER_ID);
    expect(groupOf(libraryItems([view], [])[0], READER_ID)).toBe("mine");
    expect(filed(mineList)).toBe("mine");
    expect(filed({ ...liveList, sharing: "team" })).toBe("mine");
    expect(filed({ ...liveList, sharing: "workspace" })).toBe("mine");
    expect(filed(theirs)).toBe("shared");
    expect(
      filed({ ...theirs, sharing: "private", steward_id: READER_ID }),
    ).toBe("shared");
    expect(filed({ ...shortlist, owner_id: null })).toBe("shared");
    expect(filed(shortlist)).toBe("shared");
  });

  it("counts a list as the reader's only when its owner is the known reader", () => {
    expect(ownedByReader({ owner_id: READER_ID }, READER_ID)).toBe(true);
    expect(ownedByReader({ owner_id: OTHER_OWNER_ID }, READER_ID)).toBe(false);
    expect(ownedByReader({ owner_id: null }, READER_ID)).toBe(false);
    expect(ownedByReader({}, undefined)).toBe(false);
    expect(ownedByReader({ owner_id: null }, undefined)).toBe(false);
  });

  it("files no list as the reader's while the reader is unknown", () => {
    const items = libraryItems([view], [mineList, shortlist]);
    expect(items.map((item) => groupOf(item, undefined))).toEqual([
      "mine",
      "shared",
      "shared",
    ]);
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

describe("a server search past the cap", () => {
  it("adds what it found to the rows held, once each, and drops none of them", () => {
    expect(withFound([mineList, liveList], [liveList, shortlist])).toEqual([
      mineList,
      liveList,
      shortlist,
    ]);
    expect(withFound([mineList], [])).toEqual([mineList]);
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

  it("searches the way the palette does: accents either way, words in any order", () => {
    expect(
      cutLibrary(items, { ...ALL, q: "dînner" }, captionOf).map(nameOf),
    ).toEqual([mineList.name]);
    expect(
      cutLibrary(items, { ...ALL, q: "guests october" }, captionOf).map(nameOf),
    ).toEqual([mineList.name]);
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
  const theirs = libraryItems([], [{ ...liveList, owner_id: OTHER_OWNER_ID }]);
  const drawn = (shown: readonly LibraryItem[], cut: boolean) =>
    visibleGroups(both, shown, READER_ID, cut, nothingOutstanding);

  it("are both, with no cut, so an empty one can say what goes there", () => {
    expect(drawn(libraryItems([view], []), false)).toEqual(["mine", "shared"]);
  });

  it("are only those holding rows under a cut", () => {
    expect(drawn(libraryItems([], [liveList]), true)).toEqual(["mine"]);
    expect(drawn(theirs, true)).toEqual(["shared"]);
    expect(drawn([], true)).toEqual([]);
  });

  it("include one whose read is still out or failed", () => {
    expect(
      visibleGroups(both, [], READER_ID, true, (group) => group === "shared"),
    ).toEqual(["shared"]);
  });
});

describe("where a view saved on a list opens", () => {
  const [item] = libraryItems([byName], []);
  const href = (openingFilters: Readonly<Record<string, string>>) =>
    item?.kind === "listView" ? listViewHref(item, openingFilters) : "";

  it("is that list, with the view's dials as the list writes them", () => {
    expect(href({})).toBe("#/contacts?per=25&sort=name");
  });

  it("spells out a filter the list opens on and the view leaves off", () => {
    const [allLeads] = libraryItems(
      [listView("v-all", "All leads", "leads", { sort: "", filters: {} })],
      [],
    );
    expect(
      allLeads?.kind === "listView" &&
        listViewHref(allLeads, { owner_id: READER_ID }),
    ).toBe("#/leads?owner_id=all&per=25");
  });

  it("waits for the reader before a view saved on the leads queue opens", () => {
    const [leads] = libraryItems(
      [listView("v-hot", "Hot", "leads", { sort: "-score" })],
      [],
    );
    if (leads === undefined) {
      throw new Error("no row for the leads view");
    }
    expect(hrefOf(leads, undefined)).toBeUndefined();
    expect(hrefOf(leads, { owner_id: READER_ID })).toBe(
      "#/leads?owner_id=all&per=25&sort=-score",
    );
  });

  it("opens a deals view on the table it was saved from", () => {
    const [deals] = libraryItems(
      [
        listView("v-deal", "Stalled", "deals", {
          filters: { stalled: "true", pipeline_id: "pl" },
        }),
      ],
      [],
    );
    expect(deals?.kind === "listView" && listViewHref(deals, {})).toBe(
      "#/deals?per=25&pipeline_id=pl&stalled=true&view=table",
    );
  });
});

describe("a view saved on a list, arriving at that list", () => {
  const openings = [
    { resource: "contacts", sort: CONTACTS_INITIAL_SORT, filters: {} },
    { resource: "companies", sort: COMPANIES_INITIAL_SORT, filters: {} },
    { resource: "projects", sort: PROJECTS_INITIAL_SORT, filters: {} },
    {
      resource: "leads",
      sort: LEADS_INITIAL_SORT,
      filters: leadsOpeningFilters({ user: { id: READER_ID } }),
    },
  ];
  const stored = [
    {},
    { sort: "" },
    { sort: "name" },
    { q: "acme", includeArchived: true, perPage: 100 },
    { filters: { owner_id: READER_ID } },
  ];

  it.each(openings)(
    "is read by $resource as the list the view describes, its default order included",
    ({ resource, sort, filters }) => {
      for (const list of stored) {
        const [item] = libraryItems([listView("v", "V", resource, list)], []);
        if (item?.kind !== "listView") {
          throw new Error(`no list row for ${resource}`);
        }
        const query = listQueryFromParams(
          parseParams(listViewHref(item, filters)),
          listOpening(sort, filters),
          false,
        );
        expect(matchesView(item.state, query)).toBe(true);
        expect(query.perPage).toBe(item.state.perPage ?? 25);
      }
    },
  );
});

describe("a list address", () => {
  const theirs = libraryItems([], [{ ...liveList, owner_id: OTHER_OWNER_ID }]);

  it("lands on the reader's own group when it shows one of their lists", () => {
    const shown = libraryItems([view], [liveList]);
    expect(listsLanding([...shown, ...theirs], READER_ID)).toBe("mine");
  });

  it("lands on the shared group when the reader's own shows no list", () => {
    expect(listsLanding(libraryItems([view], []), READER_ID)).toBe("shared");
    expect(listsLanding(theirs, READER_ID)).toBe("shared");
    expect(listsLanding([], READER_ID)).toBe("shared");
  });
});
