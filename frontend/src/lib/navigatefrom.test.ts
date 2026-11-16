/** @vitest-environment happy-dom */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { noteHistoryEntry } from "./historydepth";
import { closeOnto, fromHere } from "./navigatefrom";

beforeEach(() => {
  globalThis.history.replaceState(null, "", "#/deals/d1");
});

describe("fromHere", () => {
  it("moves the reader who is still where the action started", () => {
    const from = fromHere();
    const entries = globalThis.history.length;
    from.navigate({ screen: "offers", id: "o1" });
    expect(globalThis.location.hash).toBe("#/offers/o1");
    expect(globalThis.history.length).toBe(entries + 1);
  });

  it("replaces the entry the action started on when asked to", () => {
    const from = fromHere();
    const entries = globalThis.history.length;
    from.navigateReplacing({ screen: "deals" }, new Map([["stage", "won"]]));
    expect(globalThis.location.hash).toBe("#/deals?stage=won");
    expect(globalThis.history.length).toBe(entries);
  });

  it("leaves a reader who moved on during the work where they are", () => {
    const from = fromHere();
    globalThis.location.hash = "#/contacts/c1";
    from.navigate({ screen: "offers", id: "o1" });
    from.navigateReplacing({ screen: "deals" });
    expect(globalThis.location.hash).toBe("#/contacts/c1");
    expect(from.stillHere()).toBe(false);
  });

  it("moves a reader who only turned a dial on the page during the work", () => {
    const from = fromHere();
    globalThis.history.replaceState(null, "", "#/deals/d1?tab=files");
    from.navigate({ screen: "offers", id: "o1" });
    expect(globalThis.location.hash).toBe("#/offers/o1");
  });
});

describe("closeOnto", () => {
  // A run of this app's entries, each counted the way the router counts one.
  function entries(...hashes: string[]) {
    globalThis.history.replaceState(null, "", hashes[0]);
    noteHistoryEntry("start");
    for (const hash of hashes.slice(1)) {
      globalThis.history.pushState(null, "", hash);
      noteHistoryEntry("moved");
    }
  }

  // The browser accepting a traversal and not yet carrying it out.
  const goPending = () =>
    vi.spyOn(globalThis.history, "go").mockImplementation(() => undefined);

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("goes back when the entry behind is the address it closes onto", () => {
    entries("#/home?view=week", "#/home?queue=1&view=week");
    const go = goPending();
    closeOnto({ screen: "home" }, new Map([["view", "week"]]));
    expect(go.mock.calls).toEqual([[-1]]);
  });

  it("reads the bare entry as Home", () => {
    entries("#/", "#/home?queue=1");
    const go = goPending();
    closeOnto({ screen: "home" });
    expect(go.mock.calls).toEqual([[-1]]);
  });

  it("goes back once for a second close pressed before the first lands", () => {
    entries("#/home", "#/home?queue=1");
    const go = goPending();
    closeOnto({ screen: "home" });
    closeOnto({ screen: "home" });
    expect(go.mock.calls).toEqual([[-1]]);
    expect(globalThis.location.hash).toBe("#/home?queue=1");
  });

  it("puts the address in place when the entry behind is another page", () => {
    entries("#/contacts/c1", "#/home?queue=1");
    const go = goPending();
    const length = globalThis.history.length;
    closeOnto({ screen: "home" });
    expect(go).not.toHaveBeenCalled();
    expect(globalThis.location.hash).toBe("#/home");
    expect(globalThis.history.length).toBe(length);
  });
});
