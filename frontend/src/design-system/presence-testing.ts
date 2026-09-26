// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { vi } from "vitest";

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
