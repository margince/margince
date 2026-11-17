/** @vitest-environment happy-dom */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// Each case gets the modules as a fresh page load has them: the history depth
// counted, and any traversal still in flight, belong to one load.
let depth: typeof import("./historydepth");
let nav: typeof import("./navigatefrom");

beforeEach(async () => {
  vi.resetModules();
  depth = await import("./historydepth");
  nav = await import("./navigatefrom");
  globalThis.history.replaceState(null, "", "#/deals/d1");
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("fromHere", () => {
  it("moves the reader who is still where the action started", () => {
    const from = nav.fromHere();
    const entries = globalThis.history.length;
    from.navigate({ screen: "offers", id: "o1" });
    expect(globalThis.location.hash).toBe("#/offers/o1");
    expect(globalThis.history.length).toBe(entries + 1);
  });

  it("replaces the entry the action started on when asked to", () => {
    const from = nav.fromHere();
    const entries = globalThis.history.length;
    from.navigateReplacing({ screen: "deals" }, new Map([["stage", "won"]]));
    expect(globalThis.location.hash).toBe("#/deals?stage=won");
    expect(globalThis.history.length).toBe(entries);
  });

  it("leaves a reader who moved on during the work where they are", () => {
    const from = nav.fromHere();
    globalThis.location.hash = "#/contacts/c1";
    from.navigate({ screen: "offers", id: "o1" });
    from.navigateReplacing({ screen: "deals" });
    expect(globalThis.location.hash).toBe("#/contacts/c1");
    expect(from.stillHere()).toBe(false);
  });

  it("closes a record for a reader who only opened another tab of it during the work", () => {
    globalThis.history.replaceState(null, "", "#/contacts/c1");
    const from = nav.fromHere();
    globalThis.history.replaceState(null, "", "#/contacts/c1/timeline");
    from.closeOnto({ screen: "contacts" });
    expect(globalThis.location.hash).toBe("#/contacts");
  });

  it("leaves a reader who opened another record of the same list where they are", () => {
    globalThis.history.replaceState(null, "", "#/contacts/c1");
    const from = nav.fromHere();
    globalThis.history.replaceState(null, "", "#/contacts/c2");
    from.closeOnto({ screen: "contacts" });
    expect(globalThis.location.hash).toBe("#/contacts/c2");
  });

  it("leaves a reader who moved to another section where they are", () => {
    globalThis.history.replaceState(null, "", "#/analytics/reports");
    const from = nav.fromHere();
    globalThis.history.replaceState(null, "", "#/analytics/overview");
    from.navigate({ screen: "analytics", id: "reports", id2: "r1" });
    expect(globalThis.location.hash).toBe("#/analytics/overview");
  });

  it("moves a reader who only turned a dial on the page during the work", () => {
    const from = nav.fromHere();
    globalThis.history.replaceState(null, "", "#/deals/d1?tab=files");
    from.navigate({ screen: "offers", id: "o1" });
    expect(globalThis.location.hash).toBe("#/offers/o1");
  });
});

describe("closeOnto", () => {
  // A run of this app's entries, each counted the way the router counts one.
  function entries(...hashes: string[]) {
    globalThis.history.replaceState(null, "", hashes[0]);
    depth.noteHistoryEntry("start");
    for (const hash of hashes.slice(1)) {
      globalThis.history.pushState(null, "", hash);
      depth.noteHistoryEntry("moved");
    }
  }

  // The browser accepting a traversal and not yet carrying it out.
  const goPending = () =>
    vi.spyOn(globalThis.history, "go").mockImplementation(() => undefined);

  it("goes back when the entry behind is the page it closes onto", () => {
    entries("#/home?view=week", "#/home?queue=1&view=week");
    const go = goPending();
    nav.closeOnto({ screen: "home" }, new Map([["view", "week"]]));
    expect(go.mock.calls).toEqual([[-1]]);
  });

  it("goes back to the page behind with the dials the reader left on it", () => {
    entries("#/contacts?owner=me", "#/contacts/c1");
    const go = goPending();
    nav.closeOnto({ screen: "contacts" });
    expect(go.mock.calls).toEqual([[-1]]);
  });

  it("steps back over the record's own tabs to the page it closes onto", () => {
    entries("#/contacts?owner=me", "#/contacts/c1", "#/contacts/c1/timeline");
    const go = goPending();
    nav.closeOnto({ screen: "contacts" });
    expect(go.mock.calls).toEqual([[-2]]);
  });

  it("reads the bare entry as Home", () => {
    entries("#/", "#/home?queue=1");
    const go = goPending();
    nav.closeOnto({ screen: "home" });
    expect(go.mock.calls).toEqual([[-1]]);
  });

  it("goes back once for a second close pressed before the first lands", () => {
    entries("#/home", "#/home?queue=1");
    const go = goPending();
    nav.closeOnto({ screen: "home" });
    nav.closeOnto({ screen: "home" });
    expect(go.mock.calls).toEqual([[-1]]);
    expect(globalThis.location.hash).toBe("#/home?queue=1");
  });

  it("puts the address in place when the entry behind is another page", () => {
    entries("#/contacts/c1", "#/home?queue=1");
    const go = goPending();
    const length = globalThis.history.length;
    nav.closeOnto({ screen: "home" });
    expect(go).not.toHaveBeenCalled();
    expect(globalThis.location.hash).toBe("#/home");
    expect(globalThis.history.length).toBe(length);
  });
});
