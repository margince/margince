// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

import { describe, expect, it } from "vitest";
import { createEdgeRenderer, type EdgeHues } from "./agent-edge-gl";

// What this file can check without a GPU. The token seam these tests used to
// carry moved with `readHue` into the design system, which is where both shaders
// now read it from; it is pinned in `margince-core-gl.test.ts`.

const HUES: EdgeHues = [
  [0, 0, 0],
  [0, 0, 0],
  [0, 0, 0],
  [0, 0, 0],
  [0, 0, 0],
];

describe("the renderer, on a host that cannot draw", () => {
  it("reports null rather than throwing", () => {
    // jsdom has no WebGL2, which is the same answer a locked-down browser gives.
    // The caller wears a static rim, and it only gets the chance if this returns
    // rather than raising: a decoration must not be able to break a page.
    const canvas = document.createElement("canvas");

    expect(createEdgeRenderer(canvas, HUES)).toBeNull();
  });
});
