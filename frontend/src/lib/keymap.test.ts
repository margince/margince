// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  ariaKeyShortcuts,
  type BindingKeys,
  capOf,
  isCharacterKey,
  KEYMAP,
  shortcutCaps,
  shortcutTip,
} from "./keymap";

const dispatched = KEYMAP.filter((entry) => entry.answeredBy === "dispatcher");

const pressed = (keys: BindingKeys) =>
  keys.kind === "sequence"
    ? `${keys.kind} ${keys.keys.join(" ")}`
    : `${keys.kind} ${keys.key}`;

describe("the platform's modifier", () => {
  it.each(["MacIntel", "iPhone"])("is ⌘ on %s", (platform) => {
    expect(shortcutCaps("sidebar", platform)).toEqual(["⌘", "B"]);
    expect(shortcutCaps("search", platform)).toEqual(["⌘", "K"]);
  });

  // An unreported platform is more likely Windows or Linux than a Mac.
  it.each(["Win32", "Linux x86_64", ""])("is Ctrl on %j", (platform) => {
    expect(shortcutCaps("search", platform)).toEqual(["Ctrl", "K"]);
  });
});

describe("the keymap", () => {
  it("gives no two bindings the dispatcher answers the same keys", () => {
    const keys = dispatched.map((entry) => pressed(entry.keys));
    expect(new Set(keys).size).toBe(keys.length);
  });

  // A single key that also leads a sequence would wait out the window every time.
  it("binds no single key that leads a sequence", () => {
    const leads = new Set(
      dispatched.flatMap((entry) =>
        entry.keys.kind === "sequence" ? [entry.keys.keys[0]] : [],
      ),
    );
    const singles = dispatched.flatMap((entry) =>
      entry.keys.kind === "single" ? [entry.keys.key] : [],
    );
    expect(singles.filter((key) => leads.has(key))).toEqual([]);
  });

  it("counts only unmodified keys as ones a reader may turn off", () => {
    expect(
      KEYMAP.filter((entry) => isCharacterKey(entry.keys)).map(
        (entry) => entry.keys.kind,
      ),
    ).not.toContain("chord");
    expect(isCharacterKey({ kind: "sequence", keys: ["g", "c"] })).toBe(true);
    expect(isCharacterKey({ kind: "single", key: "?" })).toBe(true);
    expect(isCharacterKey({ kind: "named", key: "Escape" })).toBe(false);
  });

  it("prints Escape as the cap a keyboard carries", () => {
    expect(capOf("Escape")).toBe("Esc");
    expect(capOf("ArrowDown")).toBe("↓");
    expect(capOf("c")).toBe("C");
  });
});

describe("aria-keyshortcuts", () => {
  it("names both modifiers of a chord, and a single key as typed", () => {
    expect(ariaKeyShortcuts({ kind: "chord", key: "k" })).toBe(
      "Meta+K Control+K",
    );
    expect(ariaKeyShortcuts({ kind: "single", key: "?" })).toBe("?");
  });

  // A space separates alternatives, so "g c" would claim C alone goes there.
  it("names no sequence", () => {
    expect(
      ariaKeyShortcuts({ kind: "sequence", keys: ["g", "c"] }),
    ).toBeUndefined();
  });
});

describe("a label with its shortcut", () => {
  it("joins a chord's caps, and a sequence's with the reader's word", () => {
    expect(shortcutTip("Search", { kind: "single", key: "/" }, "then")).toBe(
      "Search · /",
    );
    expect(
      shortcutTip(undefined, { kind: "sequence", keys: ["g", "c"] }, "dann"),
    ).toBe("G dann C");
  });
});
