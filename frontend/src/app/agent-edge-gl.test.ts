// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

import { afterEach, describe, expect, it } from "vitest";
import { createEdgeRenderer, type EdgeHues, readHue } from "./agent-edge-gl";

// The one part of the renderer that is not a GPU call: the seam between the
// design tokens and the shader. A shader cannot read a custom property, so this
// is where a token becomes a uniform, and it is worth pinning because getting it
// wrong is silent — a bad parse draws a colour nobody chose rather than failing.

const HUES: EdgeHues = [
  [0, 0, 0],
  [0, 0, 0],
  [0, 0, 0],
  [0, 0, 0],
  [0, 0, 0],
];

afterEach(() => {
  document.documentElement.style.removeProperty("--test-hue");
});

describe("reading a colour token for the shader", () => {
  it("turns a token into the floats the shader wants", () => {
    document.documentElement.style.setProperty("--test-hue", "#2fbe8f");

    const [r, g, b] = readHue("--test-hue");
    expect(r).toBeCloseTo(47 / 255, 5);
    expect(g).toBeCloseTo(190 / 255, 5);
    expect(b).toBeCloseTo(143 / 255, 5);
  });

  it("answers mid-grey for a token that is not there", () => {
    // Not black. A missing hue should look WRONG rather than look like a hole:
    // black on a premultiplied canvas is indistinguishable from a rim that
    // failed to draw, and one of those is a bug worth noticing.
    expect(readHue("--no-such-token")).toEqual([0.5, 0.5, 0.5]);
  });

  it("answers mid-grey rather than guessing at a colour it cannot parse", () => {
    // Tokens in this tree are six-digit hex. A named colour or an `oklch()` is
    // not a parse failure to paper over with a channel-wise guess; it means the
    // token moved and this seam has to be told about it.
    document.documentElement.style.setProperty("--test-hue", "rebeccapurple");

    expect(readHue("--test-hue")).toEqual([0.5, 0.5, 0.5]);
  });
});

describe("the renderer, on a host that cannot draw", () => {
  it("reports null rather than throwing", () => {
    // jsdom has no WebGL2, which is the same answer a locked-down browser gives.
    // The caller wears a static rim, and it only gets the chance if this returns
    // rather than raising: a decoration must not be able to break a page.
    const canvas = document.createElement("canvas");

    expect(createEdgeRenderer(canvas, HUES)).toBeNull();
  });
});
