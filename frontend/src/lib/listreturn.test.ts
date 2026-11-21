// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// The way out of a record, pressed twice. The address store and the history
// are the real ones; each case gets them as a fresh page load has them.
let router: typeof import("./router");
let leaveRecord: typeof import("./listreturn").leaveRecord;
let leftRecord: typeof import("./listreturn").leftRecord;
let unwatch: () => void;

const DEAL = { screen: "deals", id: "d-1", id2: "activity" } as const;

beforeEach(async () => {
  vi.resetModules();
  router = await import("./router");
  ({ leaveRecord, leftRecord } = await import("./listreturn"));
  globalThis.history.replaceState(null, "", "#/home");
  unwatch = router.watchAddress(() => undefined);
  await arrive("#/deals");
  await arrive("#/deals/d-1");
  await arrive("#/deals/d-1/activity");
});

afterEach(() => {
  unwatch();
  vi.restoreAllMocks();
});

async function arrive(hash: string) {
  const moved = new Promise((resolve) =>
    globalThis.addEventListener("hashchange", resolve, { once: true }),
  );
  globalThis.location.hash = hash;
  await moved;
}

describe("leaveRecord pressed twice", () => {
  it("starts one traversal while the browser has not landed yet", () => {
    const go = vi
      .spyOn(globalThis.history, "go")
      .mockImplementation(() => undefined);

    leaveRecord(DEAL);
    leaveRecord(DEAL);

    expect(go.mock.calls).toEqual([[-2]]);
    expect(globalThis.location.hash).toBe("#/deals/d-1/activity");
    go.mockRestore();
    globalThis.history.go(-2);
    expect(globalThis.location.hash).toBe("#/deals");
  });

  it("stays on the list when the second press comes after the landing", () => {
    leaveRecord(DEAL);
    expect(globalThis.location.hash).toBe("#/deals");

    leaveRecord(DEAL);

    expect(globalThis.location.hash).toBe("#/deals");
  });
});

describe("leftRecord", () => {
  it("holds once leaving has landed on the list", () => {
    leaveRecord(DEAL);

    expect(leftRecord(DEAL, globalThis.location.hash)).toBe(true);
  });

  it("does not hold for a bar still on the record, another tab ahead", () => {
    globalThis.history.back();
    expect(globalThis.location.hash).toBe("#/deals/d-1");

    expect(leftRecord(DEAL, globalThis.location.hash)).toBe(false);
  });
});
