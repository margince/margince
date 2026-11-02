/** @vitest-environment happy-dom */
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  forgetSeat,
  removeStored,
  STORAGE_KEYS,
  watchStored,
  writeStored,
} from "./storage";

afterEach(() => {
  forgetSeat();
  vi.restoreAllMocks();
});
describe("watching a stored value", () => {
  it("reports same-tab writes, removal and seat erasure", () => {
    const changed = vi.fn();
    const stop = watchStored(STORAGE_KEYS.paletteRecents, changed);
    writeStored(STORAGE_KEYS.paletteRecents, "history");
    expect(changed).toHaveBeenCalledTimes(1);
    writeStored(STORAGE_KEYS.theme, "dark");
    expect(changed).toHaveBeenCalledTimes(1);
    forgetSeat();
    expect(changed).toHaveBeenCalledTimes(2);
    removeStored(STORAGE_KEYS.paletteRecents);
    expect(changed).toHaveBeenCalledTimes(3);
    stop();
    writeStored(STORAGE_KEYS.paletteRecents, "another");
    expect(changed).toHaveBeenCalledTimes(3);
  });
  it("reports matching storage events and whole-area clears", () => {
    const changed = vi.fn();
    const stop = watchStored(STORAGE_KEYS.paletteRecents, changed);
    globalThis.dispatchEvent(
      new StorageEvent("storage", { key: STORAGE_KEYS.theme.name }),
    );
    expect(changed).not.toHaveBeenCalled();
    globalThis.dispatchEvent(
      new StorageEvent("storage", {
        key: STORAGE_KEYS.paletteRecents.name,
        newValue: null,
      }),
    );
    globalThis.dispatchEvent(new StorageEvent("storage", { key: null }));
    expect(changed).toHaveBeenCalledTimes(2);
    stop();
  });
});

it("announces sign-out even when a seat key has not been written yet", () => {
  removeStored(STORAGE_KEYS.paletteRecents);
  const changed = vi.fn();
  const stop = watchStored(STORAGE_KEYS.paletteRecents, changed);
  forgetSeat();
  expect(changed).toHaveBeenCalledWith("remove");
  stop();
});

it("announces sign-out to a watched seat-family member that is still absent", () => {
  const changed = vi.fn();
  const stop = watchStored(
    { family: STORAGE_KEYS.latestInboundNoticed, member: "pending" },
    changed,
  );
  forgetSeat();
  expect(changed).toHaveBeenCalledWith("remove");
  stop();
});
it("ignores a clear from the other storage area", () => {
  const changed = vi.fn();
  const stop = watchStored(STORAGE_KEYS.paletteRecents, changed);
  window.dispatchEvent(
    new StorageEvent("storage", { key: null, storageArea: sessionStorage }),
  );
  expect(changed).not.toHaveBeenCalled();
  stop();
});

const seatKeys = Object.values(STORAGE_KEYS).filter(
  (key) => key.lifetime === "seat",
);
it("derives a nonempty reset census from the registered seat keys", () => {
  expect(seatKeys.length).toBeGreaterThan(0);
});
it.each(seatKeys)(
  "resets an absent watcher for a registered seat key %#",
  (key) => {
    const changed = vi.fn();
    const subject =
      "name" in key ? key : { family: key, member: "not-written" };
    const stop = watchStored(subject, changed);
    try {
      forgetSeat();
      expect(changed).toHaveBeenCalledWith("remove");
    } finally {
      stop();
    }
  },
);

it("reports only successful changes when a browser refuses writes", () => {
  const storage = localStorage;
  vi.stubGlobal("localStorage", {
    get length() {
      return storage.length;
    },
    getItem: storage.getItem.bind(storage),
    key: storage.key.bind(storage),
    removeItem: storage.removeItem.bind(storage),
    clear: storage.clear.bind(storage),
    setItem() {
      throw new DOMException("Full", "QuotaExceededError");
    },
  });
  const changed = vi.fn();
  const stop = watchStored(STORAGE_KEYS.paletteRecents, changed);
  try {
    writeStored(STORAGE_KEYS.paletteRecents, "refused");
    expect(changed).not.toHaveBeenCalled();
  } finally {
    stop();
    vi.unstubAllGlobals();
  }
});
