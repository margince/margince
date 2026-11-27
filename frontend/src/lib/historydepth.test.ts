// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// Each case gets the module as a fresh page load has it. The tab's history is
// shared, so each one also starts on an entry that carries no stamp.
let depth: typeof import("./historydepth");

beforeEach(async () => {
  vi.resetModules();
  depth = await import("./historydepth");
  globalThis.history.replaceState(null, "", "#/start");
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

// A new entry the way a link makes one: a fresh address with no state.
function push(hash: string) {
  globalThis.history.pushState(null, "", hash);
  depth.noteHistoryEntry("moved");
}

// The browser accepting a traversal and not yet carrying it out.
const goPending = () =>
  vi.spyOn(globalThis.history, "go").mockImplementation(() => undefined);

const depthBehind = () => depth.depthBackOutOf(() => false);

// The Navigation API as Chromium has it, reduced to the entry list and events.
// The entries from `documentFrom` on belong to the document now loaded.
function browserEntries(
  hashes: string[],
  at: () => number,
  documentFrom = hashes.length,
) {
  const navigation = Object.assign(new EventTarget(), {
    entries: () =>
      hashes.map((hash, index) => ({
        url: `http://localhost/${hash}`,
        sameDocument: index >= documentFrom,
      })),
  });
  Object.defineProperty(navigation, "currentEntry", {
    get: () => ({ index: at() }),
  });
  vi.stubGlobal("navigation", navigation);
  return navigation;
}

// What Chromium fires once a traversal is on its way to another document.
function documentLoadStarts(navigation: EventTarget) {
  const event = new Event("navigate");
  Object.defineProperty(event, "destination", {
    value: { sameDocument: false },
  });
  navigation.dispatchEvent(event);
}

describe("traverseTo", () => {
  it("refuses before the app has noted any entry", () => {
    const go = goPending();
    expect(depth.traverseTo(0)).toBe(false);
    expect(go).not.toHaveBeenCalled();
  });

  it("refuses the entry the reader is on and any before the app's first", () => {
    depth.noteHistoryEntry("start");
    push("#/deals");
    const go = goPending();

    expect(depth.traverseTo(1)).toBe(false);
    expect(depth.traverseTo(-1)).toBe(false);
    expect(go).not.toHaveBeenCalled();
  });

  it("starts one traversal and refuses the next until the browser lands", () => {
    depth.noteHistoryEntry("start");
    push("#/deals");
    push("#/deals/d-1");
    const go = goPending();

    expect(depth.traverseTo(0)).toBe(true);
    expect(depth.traverseTo(0)).toBe(false);
    expect(depth.traverseTo(1)).toBe(false);
    expect(go.mock.calls).toEqual([[-2]]);

    depth.noteHistoryEntry("moved");
    expect(depth.traverseTo(0)).toBe(true);
  });

  it("reports a traversal in flight until the browser lands", () => {
    depth.noteHistoryEntry("start");
    push("#/deals");
    goPending();
    expect(depth.traversalInFlight()).toBe(false);

    depth.traverseTo(0);
    expect(depth.traversalInFlight()).toBe(true);

    depth.noteHistoryEntry("moved");
    expect(depth.traversalInFlight()).toBe(false);
  });

  it("reports no traversal when the browser had no entry to go to", () => {
    browserEntries(["#/start"], () => 0);
    depth.noteHistoryEntry("start");
    goPending();

    expect(depth.traverseTo(1)).toBe(false);
    expect(depth.traversalInFlight()).toBe(false);
  });

  it("stops waiting for a traversal the browser never carries out", () => {
    vi.useFakeTimers();
    depth.noteHistoryEntry("start");
    push("#/deals");
    goPending();
    depth.traverseTo(0);
    expect(depth.traverseTo(0)).toBe(false);

    vi.runAllTimers();

    expect(depth.traverseTo(0)).toBe(true);
  });
});

describe("traverseOr", () => {
  it("calls the fallback when the browser has no entry to step back to", () => {
    browserEntries(["#/contacts"], () => 0);
    depth.noteHistoryEntry("start");
    push("#/contacts/c1");
    const go = goPending();
    const instead = vi.fn();

    depth.traverseOr(0, instead);

    expect(go).not.toHaveBeenCalled();
    expect(instead).toHaveBeenCalledOnce();
  });

  it("waits for a started step instead of racing it with the fallback", () => {
    vi.useFakeTimers();
    depth.noteHistoryEntry("start");
    push("#/contacts/c1");
    const go = goPending();
    const instead = vi.fn();

    depth.traverseOr(0, instead);
    depth.traverseOr(0, instead);
    vi.runAllTimers();

    expect(go.mock.calls).toEqual([[-1]]);
    expect(instead).not.toHaveBeenCalled();
  });
});

describe("traverseTo with the browser's entry list", () => {
  it("refuses an entry the browser no longer has", () => {
    let index = 0;
    browserEntries(["#/start", "#/contacts"], () => index);
    depth.noteHistoryEntry("start");
    index = 1;
    push("#/contacts");
    const go = goPending();

    expect(depth.traverseTo(2)).toBe(false);
    expect(go).not.toHaveBeenCalled();
    expect(depth.traverseTo(0)).toBe(true);
  });

  it("holds a traversal that loads a document until the page shows again", () => {
    vi.useFakeTimers();
    let index = 0;
    const navigation = browserEntries(["#/start", "#/deals"], () => index);
    depth.noteHistoryEntry("start");
    index = 1;
    push("#/deals");
    goPending();
    depth.traverseTo(0);

    documentLoadStarts(navigation);
    vi.runAllTimers();
    expect(depth.traverseTo(0)).toBe(false);

    globalThis.dispatchEvent(new Event("pageshow"));
    expect(depth.traverseTo(0)).toBe(true);
  });
});

describe("an entry whose state was rewritten without its stamp", () => {
  it("keeps its depth when the app replaces its address", () => {
    depth.noteHistoryEntry("start");
    push("#/deals");
    push("#/reset-password");
    globalThis.history.replaceState(null, "", "#/");

    depth.noteHistoryEntry("replaced");

    expect(depth.entryDepth()).toBe(2);
    expect(depthBehind()).toBe(1);
  });

  it("counts Back onto it as a step back where the browser reports its index", () => {
    let index = 0;
    browserEntries([], () => index);
    depth.noteHistoryEntry("start");
    index = 1;
    push("#/deals");
    globalThis.history.replaceState(null, "", "#/deals");
    index = 2;
    push("#/contacts");

    globalThis.history.back();
    index = 1;
    depth.noteHistoryEntry("moved");

    expect(depth.entryDepth()).toBe(1);
    expect(depthBehind()).toBe(0);
  });
});

describe("what the browser's own entry list adds", () => {
  it("counts two entries added before the app hears of either", () => {
    let index = 0;
    browserEntries([], () => index);
    depth.noteHistoryEntry("start");

    globalThis.history.pushState(null, "", "#/deals");
    globalThis.history.pushState(null, "", "#/deals/d-1");
    index = 2;
    depth.noteHistoryEntry("moved");

    expect(depth.entryDepth()).toBe(2);
  });

  it("names an earlier entry this page load never saw", () => {
    browserEntries(["#/deals", "#/deals/d-1", "#/deals/d-1/activity"], () => 2);
    globalThis.history.replaceState(
      { marginceDepth: { depth: 2, at: "#/deals/d-1/activity" } },
      "",
      "#/deals/d-1/activity",
    );
    depth.noteHistoryEntry("start");

    expect(depth.depthBackOutOf((hash) => hash.startsWith("#/deals/d-1"))).toBe(
      0,
    );
  });

  it("counts a new entry as one step without it", () => {
    depth.noteHistoryEntry("start");
    push("#/deals");
    expect(depth.entryDepth()).toBe(1);
  });
});

describe("the entry the app starts watching on", () => {
  it("counts the entries this document made before the app watched", () => {
    browserEntries(["#/elsewhere", "#/home", "#/settings"], () => 2, 1);
    globalThis.history.replaceState(null, "", "#/settings");
    depth.noteHistoryEntry("start");
    const go = goPending();

    expect(depth.entryDepth()).toBe(1);
    expect(depth.hashAt(0)).toBe("#/home");
    expect(depth.traverseTo(0)).toBe(true);
    expect(go.mock.calls).toEqual([[-1]]);
  });
});

describe("hashAt", () => {
  it("answers the address of each entry this page load saw", () => {
    depth.noteHistoryEntry("start");
    push("#/deals");
    push("#/deals/d-1");

    expect([0, 1, 2].map(depth.hashAt)).toEqual([
      "#/start",
      "#/deals",
      "#/deals/d-1",
    ]);
  });

  it("knows no entry outside the app's run or not yet seen", () => {
    globalThis.history.replaceState(
      { marginceDepth: { depth: 2, at: "#/deals/d-1" } },
      "",
      "#/deals/d-1",
    );
    depth.noteHistoryEntry("start");

    expect(depth.hashAt(-1)).toBeUndefined();
    expect(depth.hashAt(1)).toBeUndefined();
    expect(depth.hashAt(3)).toBeUndefined();
  });

  it("answers an entry before a reload from the browser's list", () => {
    browserEntries(["#/home", "#/deals", "#/deals/d-1"], () => 2);
    globalThis.history.replaceState(
      { marginceDepth: { depth: 2, at: "#/deals/d-1" } },
      "",
      "#/deals/d-1",
    );
    depth.noteHistoryEntry("start");

    expect(depth.hashAt(1)).toBe("#/deals");
  });
});
