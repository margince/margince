// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  ariaKeyShortcuts,
  type BindingKeys,
  capOf,
  isCharacterKey,
  KEYMAP,
  modifierCap,
  pressedLetter,
  shortcutCaps,
} from "./keymap";

const dispatched = KEYMAP.filter((entry) => entry.answeredBy === "dispatcher");

const pressed = (keys: BindingKeys) =>
  keys.kind === "sequence"
    ? `${keys.kind} ${keys.keys.join(" ")}`
    : `${keys.kind} ${keys.key}`;

describe("the platform's modifier", () => {
  it.each(["MacIntel", "iPhone"])("is ⌘ on %s", (platform) => {
    expect(modifierCap(platform)).toBe("⌘");
    expect(shortcutCaps("sidebar", platform)).toEqual(["⌘", "B"]);
    expect(shortcutCaps("search", platform)).toEqual(["⌘", "K"]);
  });

  // An unreported platform is more likely Windows or Linux than a Mac.
  it.each(["Win32", "Linux x86_64", ""])("is Ctrl on %j", (platform) => {
    expect(modifierCap(platform)).toBe("Ctrl");
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

describe("the letter a press names", () => {
  it("is the key's own letter on a Latin layout, wherever it sits", () => {
    expect(pressedLetter({ key: "z", code: "KeyY" })).toBe("z");
    expect(pressedLetter({ key: "G", code: "KeyG" })).toBe("g");
  });

  it("is the letter at the key's place on a Cyrillic or Greek layout", () => {
    expect(pressedLetter({ key: "п", code: "KeyG" })).toBe("g");
    expect(pressedLetter({ key: "γ", code: "KeyG" })).toBe("g");
  });

  it("is nothing for a key that is no letter", () => {
    expect(pressedLetter({ key: "/", code: "Slash" })).toBeNull();
    expect(pressedLetter({ key: "ж", code: "Semicolon" })).toBeNull();
    expect(pressedLetter({ key: "Enter", code: "Enter" })).toBeNull();
  });
});
