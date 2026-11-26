// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { isChordOf, pressedLetter } from "./chord";

describe("the letter a press names", () => {
  it("is the key's own letter on a Latin layout, wherever it sits", () => {
    expect(pressedLetter({ key: "z", code: "KeyY" })).toBe("z");
    expect(pressedLetter({ key: "G", code: "KeyG" })).toBe("g");
  });

  it("is the letter at the key's place on a Cyrillic or Greek layout", () => {
    expect(pressedLetter({ key: "п", code: "KeyG" })).toBe("g");
    expect(pressedLetter({ key: "γ", code: "KeyG" })).toBe("g");
  });

  it("is nothing for punctuation at a letter's place, as AZERTY puts , at M", () => {
    expect(pressedLetter({ key: ",", code: "KeyM" })).toBeNull();
    expect(pressedLetter({ key: ";", code: "KeyM" })).toBeNull();
  });

  it("is nothing for a key that is no letter", () => {
    expect(pressedLetter({ key: "/", code: "Slash" })).toBeNull();
    expect(pressedLetter({ key: "ж", code: "Semicolon" })).toBeNull();
    expect(pressedLetter({ key: "Enter", code: "Enter" })).toBeNull();
  });
});

const press = (
  key: string,
  held: Partial<Record<"metaKey" | "ctrlKey" | "altKey" | "shiftKey", boolean>>,
) => ({
  key,
  code: key.length === 1 ? `Key${key.toUpperCase()}` : key,
  metaKey: false,
  ctrlKey: false,
  altKey: false,
  shiftKey: false,
  ...held,
});

// One rule for every ⌘/Ctrl chord: the dispatcher's, a field's ⌘↵ and the
// rich text's ⌘K all call this.
describe("a platform chord", () => {
  it("takes ⌘ or Ctrl with the key", () => {
    expect(isChordOf(press("Enter", { metaKey: true }), "Enter")).toBe(true);
    expect(isChordOf(press("k", { ctrlKey: true }), "k")).toBe(true);
    expect(isChordOf(press("л", { ctrlKey: true }), "k")).toBe(false);
    expect(
      isChordOf({ ...press("л", { ctrlKey: true }), code: "KeyK" }, "k"),
    ).toBe(true);
  });

  it.each<[string, Record<string, boolean>]>([
    ["no modifier", {}],
    ["both ⌘ and Ctrl", { metaKey: true, ctrlKey: true }],
    ["Ctrl and Alt, which is AltGr", { ctrlKey: true, altKey: true }],
    ["Shift", { metaKey: true, shiftKey: true }],
  ])("refuses %s", (_, held) => {
    expect(isChordOf(press("Enter", held), "Enter")).toBe(false);
  });
});
