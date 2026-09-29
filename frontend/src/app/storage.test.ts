// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it, vi } from "vitest";
import {
  forgetSeat,
  readStored,
  readStoredJson,
  removeStored,
  STORAGE_KEYS,
  writeStored,
} from "./storage";

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  localStorage.clear();
  sessionStorage.clear();
});

function words(value: unknown): string[] | null {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : null;
}

// Throws on every call, the way a locked-down profile's Storage does.
function refusingStorage(error: DOMException): Storage {
  const refuse = () => {
    throw error;
  };
  return {
    length: 0,
    clear: refuse,
    getItem: refuse,
    key: refuse,
    removeItem: refuse,
    setItem: refuse,
  };
}

function storedNames(storage: Storage): string[] {
  return Array.from({ length: storage.length }, (_, index) =>
    storage.key(index),
  )
    .filter((name) => name !== null)
    .sort();
}

describe("a stored value", () => {
  it("reads back what was written, under the name earlier builds wrote", () => {
    writeStored(STORAGE_KEYS.theme, "dark");

    expect(readStored(STORAGE_KEYS.theme)).toBe("dark");
    expect(localStorage.getItem("margince.theme")).toBe("dark");

    removeStored(STORAGE_KEYS.theme);

    expect(readStored(STORAGE_KEYS.theme)).toBeNull();
  });

  it("lives in the area its key declares", () => {
    writeStored(STORAGE_KEYS.oauthAttempt, "gmail");

    expect(sessionStorage.getItem("ob.connect.oauthAttempt")).toBe("gmail");
    expect(localStorage.getItem("ob.connect.oauthAttempt")).toBeNull();
  });

  it("keeps a family member under the family's prefix and its own suffix", () => {
    const leads = { family: STORAGE_KEYS.tableWidths, member: "leads" };

    writeStored(leads, '{"name":240}');

    expect(localStorage.getItem("margince.table.widths.v2.leads")).toBe(
      '{"name":240}',
    );
    expect(readStoredJson(leads, (value) => value)).toEqual({ name: 240 });
  });

  it("reads JSON as the caller's parser shapes it, and anything else as absent", () => {
    expect(readStoredJson(STORAGE_KEYS.faultsSeen, words)).toBeNull();

    writeStored(STORAGE_KEYS.faultsSeen, '["run-1","run-2"]');
    expect(readStoredJson(STORAGE_KEYS.faultsSeen, words)).toEqual([
      "run-1",
      "run-2",
    ]);

    writeStored(STORAGE_KEYS.faultsSeen, "{not json");
    expect(readStoredJson(STORAGE_KEYS.faultsSeen, words)).toBeNull();

    writeStored(STORAGE_KEYS.faultsSeen, '{"run-1":true}');
    expect(readStoredJson(STORAGE_KEYS.faultsSeen, words)).toBeNull();
  });

  // Renaming a key strands every value a reader already has under the old one.
  it("keeps every name byte-identical to what shipped", () => {
    expect(
      Object.values(STORAGE_KEYS).map((key) =>
        "name" in key ? key.name : `${key.prefix}*`,
      ),
    ).toEqual([
      "margince.locale",
      "margince.theme",
      "margince.sidebarCollapsed",
      "margince.pageAside.collapsed",
      "margince.edgeLight",
      "margince.table.widths.v2.*",
      "margince.agent.faults-seen",
      "margince.leads.segregationNoteDismissed",
      "margince.import.run",
      "margince.first-run.platform-declined:*",
      "ob.connect.oauthAttempt",
      "ob.connect.overnightChoice",
      "margince.room.session",
    ]);
  });
});

describe("a browser that refuses storage", () => {
  it("answers absent when merely reaching for storage throws", () => {
    vi.spyOn(globalThis, "localStorage", "get").mockImplementation(() => {
      throw new DOMException("The operation is insecure.", "SecurityError");
    });

    expect(readStored(STORAGE_KEYS.locale)).toBeNull();
    expect(readStoredJson(STORAGE_KEYS.faultsSeen, words)).toBeNull();
    expect(() => writeStored(STORAGE_KEYS.locale, "de")).not.toThrow();
    expect(() => removeStored(STORAGE_KEYS.locale)).not.toThrow();
    expect(() => forgetSeat()).not.toThrow();
  });

  it("drops a write the quota refuses, and every other call answers absent", () => {
    vi.stubGlobal(
      "localStorage",
      refusingStorage(new DOMException("full", "QuotaExceededError")),
    );

    expect(() => writeStored(STORAGE_KEYS.theme, "dark")).not.toThrow();
    expect(readStored(STORAGE_KEYS.theme)).toBeNull();
    expect(() => removeStored(STORAGE_KEYS.theme)).not.toThrow();
  });

  it("answers absent in a runtime with no Web Storage at all", () => {
    vi.stubGlobal("sessionStorage", undefined);

    expect(readStored(STORAGE_KEYS.buyerRoomSession)).toBeNull();
    expect(() =>
      writeStored(STORAGE_KEYS.buyerRoomSession, "mdrs_token"),
    ).not.toThrow();
  });
});

describe("forgetting a seat", () => {
  // Seeded from the registry itself, so a key added later is judged here too.
  function seedEveryKey(): void {
    for (const key of Object.values(STORAGE_KEYS)) {
      const store = key.area === "local" ? localStorage : sessionStorage;
      if ("name" in key) {
        store.setItem(key.name, "kept");
      } else {
        store.setItem(`${key.prefix}first`, "kept");
        store.setItem(`${key.prefix}second`, "kept");
      }
    }
  }

  function survivingNames(area: "local" | "session"): string[] {
    return Object.values(STORAGE_KEYS)
      .filter((key) => key.area === area && key.lifetime !== "seat")
      .flatMap((key) =>
        "name" in key
          ? [key.name]
          : [`${key.prefix}first`, `${key.prefix}second`],
      );
  }

  it("erases every seat key and family member, in both areas, and keeps the rest", () => {
    seedEveryKey();
    localStorage.setItem("another.app.key", "theirs");

    forgetSeat();

    expect(storedNames(localStorage)).toEqual(
      [...survivingNames("local"), "another.app.key"].sort(),
    );
    expect(storedNames(sessionStorage)).toEqual(
      survivingNames("session").sort(),
    );
    expect(readStored(STORAGE_KEYS.theme)).toBe("kept");
    expect(readStored(STORAGE_KEYS.importRun)).toBeNull();
    expect(readStored(STORAGE_KEYS.buyerRoomSession)).toBeNull();
    // Named by its account, so the next reader cannot inherit it.
    expect(
      readStored({ family: STORAGE_KEYS.platformDeclined, member: "first" }),
    ).toBe("kept");
  });

  it("still clears the area that answers when the other refuses", () => {
    seedEveryKey();
    vi.spyOn(globalThis, "localStorage", "get").mockImplementation(() => {
      throw new DOMException("The operation is insecure.", "SecurityError");
    });

    forgetSeat();

    expect(storedNames(sessionStorage)).toEqual(
      survivingNames("session").sort(),
    );
  });
});
