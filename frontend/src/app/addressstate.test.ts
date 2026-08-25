// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from "vitest";
import { navigateReplacing } from "./router";
import { replaceParams } from "./urlstate";

/**
 * Every in-app write that REPLACES the reader's history entry has to leave that
 * entry's state object alone.
 *
 * Nothing in the router keeps state there — the hash is the router's state — so
 * `location.replace` read as the obvious spelling for years. It is not: it
 * discards the state object, and app/scrollmemory.ts stamps the entry's identity
 * in it. Every dial a reader turned, and every screen that normalised its own
 * address on arrival, therefore threw away the name of the place they were
 * standing in, and the offset filed under that name could never be found again.
 *
 * Two writers, one obligation, so both are pinned here rather than each beside
 * itself: a fix to one and not the other looks like a fix and restores nothing.
 */
const dials = (entries: Record<string, string>) =>
  new Map(Object.entries(entries));

beforeEach(() => {
  window.history.replaceState({ marginceEntry: "e7" }, "", "#/companies");
});

describe("a write that replaces the entry", () => {
  it("keeps what the entry carries, when a dial moves", () => {
    replaceParams(dials({ q: "acme", sort: "full_name" }));
    expect(window.location.hash).toBe("#/companies?q=acme&sort=full_name");
    expect(window.history.state).toEqual({ marginceEntry: "e7" });
  });

  it("keeps what the entry carries, when a screen redirects", () => {
    navigateReplacing({ screen: "companies", id: "c-1", id2: "tasks" });
    expect(window.location.hash).toBe("#/companies/c-1/tasks");
    expect(window.history.state).toEqual({ marginceEntry: "e7" });
  });

  it("adds no entry, which is the whole point of replacing", () => {
    const before = window.history.length;
    replaceParams(dials({ q: "acme" }));
    navigateReplacing({ screen: "deals" });
    expect(window.history.length).toBe(before);
  });

  it("writes nothing at all when the dials have not moved", () => {
    // A search box that settles back where it started must not cost a write per
    // keystroke, and an entry whose state was never stamped must not be stamped
    // by a no-op.
    window.history.replaceState(null, "", "#/companies?q=acme");
    replaceParams(dials({ q: "acme" }));
    expect(window.history.state).toBeNull();
    expect(window.location.hash).toBe("#/companies?q=acme");
  });
});
