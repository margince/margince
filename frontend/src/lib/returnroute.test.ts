/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  keepReturnRoute,
  OIDC_FAILURE_HASH,
  restoreReturnRoute,
  takeReturnRoute,
} from "./returnroute";
import { readStored, STORAGE_KEYS, writeStored } from "./storage";

// The email link a reader follows before a provider sign-in.
const APPROVAL_LINK = "#/home?approval=ap-7";
const KEPT_AT = new Date("2026-10-22T09:00:00Z");

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(KEPT_AT);
});

afterEach(() => {
  vi.useRealTimers();
  sessionStorage.clear();
  globalThis.history.replaceState(null, "", "/");
});

function landOn(hash: string): void {
  globalThis.history.replaceState(null, "", `/${hash}`);
}

function minutesLater(minutes: number): void {
  vi.setSystemTime(KEPT_AT.getTime() + minutes * 60 * 1000);
}

describe("the route kept across a provider sign-in", () => {
  it("restores the followed link on the bare landing, then forgets it", () => {
    landOn(APPROVAL_LINK);
    keepReturnRoute();

    minutesLater(3);
    landOn("");
    restoreReturnRoute();
    expect(globalThis.location.hash).toBe(APPROVAL_LINK);
    expect(globalThis.location.pathname).toBe("/");

    landOn("");
    restoreReturnRoute();
    expect(globalThis.location.hash).toBe("");
  });

  it("drops a route kept for an attempt the reader gave up on", () => {
    landOn(APPROVAL_LINK);
    keepReturnRoute();

    minutesLater(16);
    landOn("");
    restoreReturnRoute();

    expect(globalThis.location.hash).toBe("");
    expect(readStored(STORAGE_KEYS.signInReturnRoute)).toBeNull();
  });

  it("keeps nothing for a bare entry, so a stale route cannot come back", () => {
    landOn("#/deals/d-1");
    keepReturnRoute();
    landOn("");
    keepReturnRoute();

    restoreReturnRoute();
    expect(globalThis.location.hash).toBe("");
  });

  it("spends the route when the landing names a route of its own", () => {
    landOn(APPROVAL_LINK);
    keepReturnRoute();

    landOn("#/deals/d-1");
    restoreReturnRoute();
    expect(globalThis.location.hash).toBe("#/deals/d-1");
    expect(readStored(STORAGE_KEYS.signInReturnRoute)).toBeNull();
  });

  it("leaves the route for the sign-in screen when the sign-in failed", () => {
    landOn(APPROVAL_LINK);
    keepReturnRoute();

    landOn(OIDC_FAILURE_HASH);
    restoreReturnRoute();
    expect(globalThis.location.hash).toBe(OIDC_FAILURE_HASH);
    expect(takeReturnRoute()).toBe(APPROVAL_LINK);
  });

  it("never keeps the reset screen, which renders ahead of the session", () => {
    landOn("#/reset-password");
    keepReturnRoute();
    expect(readStored(STORAGE_KEYS.signInReturnRoute)).toBeNull();
  });

  it.each([
    "https://evil.example/#/home",
    "//evil.example/#/home",
    "//home",
    "home",
    "javascript:alert(1)",
    "#/no-such-screen",
    "#/reset-password",
    "#/",
  ])("never follows a stored %o", (route) => {
    writeStored(
      STORAGE_KEYS.signInReturnRoute,
      JSON.stringify({ route, keptAt: KEPT_AT.getTime() }),
    );
    landOn("");

    restoreReturnRoute();

    expect(globalThis.location.href).toBe(`${globalThis.location.origin}/`);
    expect(readStored(STORAGE_KEYS.signInReturnRoute)).toBeNull();
  });

  it.each([
    APPROVAL_LINK,
    '{"route":"#/home"}',
    '{"route":"#/home","keptAt":"soon"}',
    "{not json",
  ])("reads a stored %o in another shape as nothing kept", (stored) => {
    writeStored(STORAGE_KEYS.signInReturnRoute, stored);
    expect(takeReturnRoute()).toBeNull();
  });

  // Restoring after the first render would show Home first and the link after.
  it("runs at boot, before the app first renders", () => {
    const main = readFileSync(resolve(__dirname, "../main.tsx"), "utf8");
    const restore = main.indexOf("restoreReturnRoute();");
    expect(restore).toBeGreaterThan(-1);
    expect(restore).toBeLessThan(main.indexOf("createRoot(root)"));
  });
});
