// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, vi } from "vitest";

/**
 * Hold every exit open for the rest of a case: a surface that closes stays
 * mounted, `inert` and closing, which is the window a reader can still act in.
 *
 * `usePresence` reads only whether an animation can end and when it did, so the
 * double is exactly those two. The caller hands the spy back with
 * `mockRestore()`.
 */
export function holdExits() {
  return vi.spyOn(HTMLElement.prototype, "getAnimations").mockReturnValue([
    {
      finished: new Promise<void>(() => undefined),
      effect: { getComputedTiming: () => ({ iterations: 1 }) },
    } as unknown as Animation,
  ]);
}

/**
 * Record each `aria-hidden="true"` that `setAttribute` writes, and whether the
 * element held focus then. The caller stops it with `restore()`.
 */
export function watchHiding() {
  const write = Element.prototype.setAttribute;
  const hidings: { element: Element; heldFocus: boolean }[] = [];
  const spy = vi
    .spyOn(Element.prototype, "setAttribute")
    .mockImplementation(function (this: Element, name, value) {
      if (name === "aria-hidden" && String(value) === "true") {
        hidings.push({
          element: this,
          heldFocus: this.contains(document.activeElement),
        });
      }
      write.call(this, name, value);
    });
  return {
    hidings,
    restore: () => spy.mockRestore(),
    // No element held focus as it was hidden. The overlay was hidden, so the
    // first check cannot pass on a case that hid nothing.
    expectHiddenAfterFocusLeft(overlay: Element) {
      expect(hidings.filter((write) => write.heldFocus)).toEqual([]);
      expect(hidings.map((write) => write.element)).toContain(overlay);
      expect(overlay.hasAttribute("inert")).toBe(true);
    },
  };
}
