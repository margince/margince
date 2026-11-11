// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { afterEach, describe, expect, it, vi } from "vitest";
import { deviceSwitch } from "./devicepreference";
import { STORAGE_KEYS } from "./storage";

const KEY = STORAGE_KEYS.singleKeyShortcuts;

afterEach(() => {
  window.localStorage.removeItem(KEY.name);
});

/** What another tab's write looks like to this one. */
function anotherTabWrites(value: string) {
  window.localStorage.setItem(KEY.name, value);
  window.dispatchEvent(
    new StorageEvent("storage", { key: KEY.name, newValue: value }),
  );
}

describe("a per-browser switch", () => {
  it("is on until a reader turns it off", () => {
    const choice = deviceSwitch(KEY);
    expect(choice.read()).toBe(true);
    choice.write(false);
    expect(choice.read()).toBe(false);
  });

  it("follows a flip made in another tab, and tells whoever listens", () => {
    const choice = deviceSwitch(KEY);
    const heard = vi.fn();
    choice.subscribe(heard);
    expect(choice.read()).toBe(true);

    anotherTabWrites("off");
    expect(choice.read()).toBe(false);
    expect(heard).toHaveBeenCalled();

    anotherTabWrites("on");
    expect(choice.read()).toBe(true);
  });
});
