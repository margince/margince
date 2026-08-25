// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import {
  LIST_PAGE_SIZES,
  type ListQuery,
  listQueryFromParams,
  paramsFromListQuery,
} from "./listquery";

// What the list would show a reader who has expressed nothing: the screen's own
// opening state. Every case below reads an address against this, because the
// question the address answers is always "what, if anything, did the reader
// change about it".
const OPENING: ListQuery = {
  q: "",
  sort: "-created_at",
  includeArchived: false,
  filters: {},
  perPage: LIST_PAGE_SIZES[0],
};

const address = (query: string) =>
  new Map(
    [...new URLSearchParams(query)].filter(([, value]) => value !== ""),
  ) as ReadonlyMap<string, string>;

/** The list a reader who has already been given an address is looking at. */
const shown = (query: string) =>
  listQueryFromParams(address(query), OPENING, true);

describe("an address a reader was sent", () => {
  it("narrows the list the way the sender had it", () => {
    expect(shown("q=acme&owner_id=u-1&sort=full_name")).toEqual({
      ...OPENING,
      q: "acme",
      sort: "full_name",
      filters: { owner_id: "u-1" },
    });
  });

  it("treats every key it does not own as a filter, so a new chip needs no code here", () => {
    // Filter keys ARE wire parameter names, spread straight onto the request.
    // A table of known filters kept here would be a second copy of every
    // screen's chip set, and a chip added to a screen would silently stop being
    // addressable until somebody remembered to extend it.
    expect(
      shown("size_band=51_200&stalled=true&partner_sourced=true").filters,
    ).toEqual({
      size_band: "51_200",
      stalled: "true",
      partner_sourced: "true",
    });
  });

  it("shows archived rows only when the address asks for them", () => {
    expect(shown("include_archived=true").includeArchived).toBe(true);
    expect(shown("q=acme").includeArchived).toBe(false);
  });

  it("opens at a page size it recognises, and ignores one it does not", () => {
    // An address is text a person can edit. `per=7` may not become a request:
    // listFetchLimit divides by it.
    expect(shown("per=100").perPage).toBe(100);
    expect(shown("per=7").perPage).toBe(OPENING.perPage);
    expect(shown("per=banana").perPage).toBe(OPENING.perPage);
  });

  it("keeps a filter the screen has never heard of rather than dropping it", () => {
    // A link made by a newer release, or hand-edited. Dropping the parameter
    // would show a list that quietly disagrees with the address it is at; the
    // server answers 422 for one it does not take, which the table renders.
    expect(shown("no_such_param=1").filters).toEqual({ no_such_param: "1" });
  });
});

describe("what a bare address means", () => {
  it("is the screen's opening state before the list has spelled that out", () => {
    const opensOnMine = { ...OPENING, filters: { owner_id: "me" } };
    expect(listQueryFromParams(new Map(), opensOnMine, false)).toEqual(
      opensOnMine,
    );
  });

  it("is an empty list state once it has", () => {
    // The reader cleared the last filter. Reading the opening filter back out
    // of the emptiness would undo the click they just made — which is what the
    // owner chip's "all" does on a queue that opens on a rep's own leads.
    const opensOnMine = { ...OPENING, filters: { owner_id: "me" } };
    const cleared = listQueryFromParams(new Map(), opensOnMine, true);
    expect(cleared.filters).toEqual({});
    expect(cleared.sort).toBe("");
  });
});

describe("the address a list writes", () => {
  it("leaves out every dial still at its default", () => {
    // A shared link is about what the reader CHOSE. Carrying the page size and
    // include_archived=false says the same thing while hiding it, and invites
    // the next reader to think three dials were turned.
    expect([...paramsFromListQuery(OPENING, OPENING).keys()]).toEqual(["sort"]);
  });

  it("spells the sort out even when it is the screen's own", () => {
    // Absence has to mean "no sort", because an unsorted list is a state a
    // reader reaches — a saved view naming no sort asks for the server's order
    // — and absence cannot spell that AND "however this list opens".
    expect(paramsFromListQuery(OPENING, OPENING).get("sort")).toBe(
      "-created_at",
    );
    const unsorted = { ...OPENING, sort: "" };
    expect(paramsFromListQuery(unsorted, OPENING).has("sort")).toBe(false);
    expect(
      listQueryFromParams(paramsFromListQuery(unsorted, OPENING), OPENING, true)
        .sort,
    ).toBe("");
  });

  it("round-trips a narrowed list, so Back lands on what the reader left", () => {
    // The whole promise: filter a list, open a record, press Back. The entry
    // Back returns to is this address, and it has to rebuild the same query.
    const narrowed: ListQuery = {
      q: "A&B Ltd",
      sort: "full_name",
      includeArchived: true,
      filters: { owner_id: "u-1", size_band: "51_200" },
      perPage: 100,
    };
    expect(
      listQueryFromParams(
        paramsFromListQuery(narrowed, OPENING),
        OPENING,
        true,
      ),
    ).toEqual(narrowed);
  });
});
