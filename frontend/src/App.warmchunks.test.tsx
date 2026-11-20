// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  type ScreenChunks,
  screenChunksTag,
  screenCodeSpecifiers,
} from "../scripts/vite-screen-chunks";
import { SCREEN_CODE } from "./App.screencode";
import { useWarmRouteChunks, WARM_ORDER } from "./App.warmchunks";

type IdleCallback = () => void;

// A hand-run idle queue, so each spec says when the browser went idle.
function stubIdle() {
  const pending = new Map<number, IdleCallback>();
  let nextHandle = 1;
  const cancel = vi.fn((handle: number) => pending.delete(handle));
  vi.stubGlobal(
    "requestIdleCallback",
    vi.fn((callback: IdleCallback) => {
      pending.set(nextHandle, callback);
      nextHandle += 1;
      return nextHandle - 1;
    }),
  );
  vi.stubGlobal("cancelIdleCallback", cancel);
  return {
    cancel,
    goIdle: async () => {
      await act(async () => {
        for (const [handle, callback] of [...pending]) {
          pending.delete(handle);
          callback();
        }
      });
    },
  };
}

// The shell's map as the build plugin writes it, one file per screen and a shared one.
function emit(screens: readonly string[]) {
  const map: ScreenChunks = { files: ["/assets/shared.js"], screens: {} };
  for (const screen of screens) {
    map.files.push(`/assets/${screen}.js`);
    map.screens[screen] = [map.files.length - 1, 0];
  }
  const tag = screenChunksTag(map);
  const block = document.createElement("script");
  for (const [name, value] of Object.entries(tag.attrs ?? {}))
    block.setAttribute(name, String(value));
  block.textContent = typeof tag.children === "string" ? tag.children : "";
  document.head.append(block);
}

const preloads = () =>
  [
    ...document.head.querySelectorAll<HTMLLinkElement>(
      'link[rel="modulepreload"]',
    ),
  ].map((link) => new URL(link.href).pathname);

async function settle(outcome: "load" | "error" = "load") {
  await act(async () => {
    for (const link of document.head.querySelectorAll("link"))
      link.dispatchEvent(new Event(outcome));
  });
}

beforeEach(() => {
  document.head.replaceChildren();
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("the warm-up order", () => {
  it("opens with deals, contacts, the contact page, companies and leads", () => {
    expect(WARM_ORDER.slice(0, 5)).toEqual([
      "deals",
      "contacts",
      "contactPage",
      "companies",
      "leads",
    ]);
  });

  it("leaves out the screens most sessions never open, and names each other screen once", () => {
    const left = Object.keys(SCREEN_CODE).filter(
      (key) => !WARM_ORDER.some((warmed) => warmed === key),
    );
    expect(new Set(left)).toEqual(
      new Set(["settings", "book", "oauthConsent", "unsubscribe", "confirm"]),
    );
    expect(new Set(WARM_ORDER).size).toBe(WARM_ORDER.length);
  });

  // The build maps every key it reads from this file; the list names only such keys.
  it("names only screens the build reads from App.screencode.ts", async () => {
    const read = await screenCodeSpecifiers(
      readFileSync(join(__dirname, "App.screencode.ts"), "utf8"),
    );
    expect([...read.keys()]).toEqual(Object.keys(SCREEN_CODE));
    for (const key of WARM_ORDER) expect(read.has(key), key).toBe(true);
  });
});

describe("useWarmRouteChunks", () => {
  it("waits for idle, then preloads one screen at a time in order", async () => {
    const idle = stubIdle();
    emit(["leads", "deals"]);

    renderHook(() => useWarmRouteChunks());
    expect(preloads()).toEqual([]);

    await idle.goIdle();
    expect(preloads()).toEqual(["/assets/deals.js", "/assets/shared.js"]);
    await idle.goIdle();
    expect(preloads()).toHaveLength(2);

    await settle();
    await idle.goIdle();
    expect(preloads()).toEqual([
      "/assets/deals.js",
      "/assets/shared.js",
      "/assets/leads.js",
    ]);
  });

  it("never preloads a screen left out of the order", async () => {
    const idle = stubIdle();
    emit(["settings", "book", "deals"]);

    renderHook(() => useWarmRouteChunks());
    for (let round = 0; round < 4; round++) {
      await idle.goIdle();
      await settle();
    }

    expect(preloads()).toEqual(["/assets/deals.js", "/assets/shared.js"]);
  });

  it("adds no second link for a file the document already preloads", async () => {
    const idle = stubIdle();
    emit(["deals"]);
    const addressed = document.createElement("link");
    addressed.rel = "modulepreload";
    addressed.href = "/assets/deals.js";
    document.head.append(addressed);

    renderHook(() => useWarmRouteChunks());
    await idle.goIdle();

    expect(preloads()).toEqual(["/assets/deals.js", "/assets/shared.js"]);
  });

  it("logs a file that fails to preload and goes on to the next screen", async () => {
    const idle = stubIdle();
    const warn = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    emit(["deals", "contacts"]);

    renderHook(() => useWarmRouteChunks());
    await idle.goIdle();
    await settle("error");
    await idle.goIdle();

    expect(warn).toHaveBeenCalledWith(
      "route chunk warm-up failed",
      expect.stringContaining("/assets/deals.js"),
    );
    expect(preloads()).toContain("/assets/contacts.js");
  });

  it("does nothing where the shell carries no map, or on a coarse pointer", async () => {
    const idle = stubIdle();
    const warn = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    renderHook(() => useWarmRouteChunks());
    await idle.goIdle();
    expect(preloads()).toEqual([]);

    emit(["deals"]);
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: query === "(pointer: coarse)",
    }));
    renderHook(() => useWarmRouteChunks());
    await idle.goIdle();
    expect(preloads()).toEqual([]);
    expect(warn).not.toHaveBeenCalled();
  });

  it("cancels the idle request when the shell unmounts first", async () => {
    const idle = stubIdle();
    emit(["deals"]);

    const { unmount } = renderHook(() => useWarmRouteChunks());
    unmount();
    await idle.goIdle();

    expect(idle.cancel).toHaveBeenCalledOnce();
    expect(preloads()).toEqual([]);
  });

  it("stops between screens once unmounted", async () => {
    const idle = stubIdle();
    emit(["deals", "contacts"]);

    const { unmount } = renderHook(() => useWarmRouteChunks());
    await idle.goIdle();
    unmount();
    await settle();
    await idle.goIdle();

    expect(preloads()).not.toContain("/assets/contacts.js");
  });

  it("falls back to a timer where the browser has no idle callback", async () => {
    vi.useFakeTimers();
    vi.stubGlobal("requestIdleCallback", undefined);
    emit(["deals"]);

    renderHook(() => useWarmRouteChunks());

    await act(async () => vi.runAllTimersAsync());
    expect(preloads()).toContain("/assets/deals.js");
  });
});
