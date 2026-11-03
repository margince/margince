// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { isSidebarChord, modifierCap, sidebarHotkeyCaps } from "./hotkeys";

const press = (
  key: string,
  held: Partial<
    Record<"metaKey" | "ctrlKey" | "altKey" | "shiftKey", boolean>
  > = {},
) => ({
  key,
  metaKey: false,
  ctrlKey: false,
  altKey: false,
  shiftKey: false,
  ...held,
});

describe("the platform's modifier", () => {
  it.each(["MacIntel", "iPhone"])("is ⌘ on %s", (platform) => {
    expect(modifierCap(platform)).toBe("⌘");
    expect(sidebarHotkeyCaps(platform)).toEqual(["⌘", "B"]);
  });

  it.each(["Win32", "Linux x86_64"])("is Ctrl on %s", (platform) => {
    expect(modifierCap(platform)).toBe("Ctrl");
    expect(sidebarHotkeyCaps(platform)).toEqual(["Ctrl", "B"]);
  });
});

describe("the sidebar chord", () => {
  it("takes B with one platform modifier", () => {
    expect(isSidebarChord(press("b", { metaKey: true }))).toBe(true);
    expect(isSidebarChord(press("b", { ctrlKey: true }))).toBe(true);
  });

  // Cmd+Shift+B is bold with a selection in most editors.
  it("refuses B with any other modifier, or none", () => {
    expect(isSidebarChord(press("B", { metaKey: true, shiftKey: true }))).toBe(
      false,
    );
    expect(isSidebarChord(press("b", { ctrlKey: true, metaKey: true }))).toBe(
      false,
    );
    expect(isSidebarChord(press("b", { altKey: true, ctrlKey: true }))).toBe(
      false,
    );
    expect(isSidebarChord(press("b"))).toBe(false);
  });
});
