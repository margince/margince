/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { readStored, STORAGE_KEYS, writeStored } from "../../lib/storage";
import { keepReturnRoute, restoreReturnRoute } from "./returnroute";

// The email link a reader follows before a provider sign-in.
const APPROVAL_LINK = "#/home?approval=ap-7";

afterEach(() => {
  sessionStorage.clear();
  globalThis.history.replaceState(null, "", "/");
});

function landOn(hash: string): void {
  globalThis.history.replaceState(null, "", `/${hash}`);
}

describe("the route kept across a provider sign-in", () => {
  it("restores the followed link on the bare landing, then forgets it", () => {
    landOn(APPROVAL_LINK);
    keepReturnRoute();

    landOn("");
    restoreReturnRoute();
    expect(globalThis.location.hash).toBe(APPROVAL_LINK);
    expect(globalThis.location.pathname).toBe("/");

    landOn("");
    restoreReturnRoute();
    expect(globalThis.location.hash).toBe("");
  });

  it("keeps nothing for a bare entry, so a stale route cannot come back", () => {
    landOn("#/deals/d-1");
    keepReturnRoute();
    landOn("");
    keepReturnRoute();

    restoreReturnRoute();
    expect(globalThis.location.hash).toBe("");
  });

  it("spends the route when the landing already names one", () => {
    landOn(APPROVAL_LINK);
    keepReturnRoute();

    landOn("#/login?oidc=failed");
    restoreReturnRoute();
    expect(globalThis.location.hash).toBe("#/login?oidc=failed");
    expect(readStored(STORAGE_KEYS.signInReturnRoute)).toBeNull();
  });

  it.each([
    "https://evil.example/#/home",
    "//evil.example/#/home",
    "//home",
    "home",
    "javascript:alert(1)",
    "#/no-such-screen",
    "#/",
  ])("never follows a stored %o", (stored) => {
    writeStored(STORAGE_KEYS.signInReturnRoute, stored);
    landOn("");

    restoreReturnRoute();

    expect(globalThis.location.href).toBe(`${globalThis.location.origin}/`);
    expect(readStored(STORAGE_KEYS.signInReturnRoute)).toBeNull();
  });

  // Restoring after the first render would show Home first and the link after.
  it("runs at boot, before the app first renders", () => {
    const main = readFileSync(resolve(__dirname, "../../main.tsx"), "utf8");
    const restore = main.indexOf("restoreReturnRoute();");
    expect(restore).toBeGreaterThan(-1);
    expect(restore).toBeLessThan(main.indexOf("createRoot(root)"));
  });
});
