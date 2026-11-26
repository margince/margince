// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

import { afterEach, describe, expect, it } from "vitest";
import { readHue } from "./margince-core-color";
import { POINT_CAP_CSS, pointGrid, pointsFor } from "./margince-core-gl";

// The seam between the design tokens and a shader, which both the Core and the
// lit window edge read through. A shader cannot read a custom property, so this
// is where a token becomes a uniform, and it is worth pinning because getting it
// wrong is SILENT: a bad parse draws a colour nobody chose rather than failing.

afterEach(() => {
  document.documentElement.style.removeProperty("--test-hue");
});

describe("reading a colour token for a shader", () => {
  it("turns a token into the floats a shader wants", () => {
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

describe("the points for a canvas size", () => {
  const count = (g: { along: number; across: number }) =>
    4 * g.along * g.across;

  it.each([22, 30, 38, 44, 50, 56, 79])(
    "keeps the small-orb grid at %i CSS px",
    (css) => {
      for (const dpr of [1, 2]) {
        expect(pointsFor(css, css * dpr)).toEqual({
          ...pointGrid(css * dpr),
          soft: 0,
        });
      }
    },
  );

  it("ramps the large treatment in from 80 to 112 CSS px", () => {
    expect(pointsFor(80, 160).soft).toBe(0);
    expect(pointsFor(96, 192).soft).toBe(0.5);
    expect(pointsFor(112, 224).soft).toBe(1);
    expect(pointsFor(300, 600).soft).toBe(1);
  });

  it("never draws a point larger than the panel's, at any large size", () => {
    for (let css = 80; css <= 320; css += 7) {
      for (const dpr of [1, 1.5, 2]) {
        const side = Math.round(css * dpr);
        expect(pointsFor(css, side).pointPx).toBeLessThanOrEqual(
          POINT_CAP_CSS * (side / css) + 1e-9,
        );
      }
    }
  });

  it("gives a capped orb more points rather than larger ones", () => {
    // 224 CSS px is the onboarding glass: uncapped, its points would be 8 px.
    const capped = pointsFor(224, 448);
    const uncapped = pointGrid(448, 1.8, 1.5);
    expect(uncapped.pointPx).toBeGreaterThan(capped.pointPx);
    expect(capped.pointPx).toBeCloseTo(POINT_CAP_CSS * 2, 9);
    expect(count(capped)).toBeGreaterThan(count(uncapped));
  });

  it("grows the count with the area, so grain spacing holds", () => {
    const panel = count(pointsFor(122, 244));
    const signIn = count(pointsFor(152, 304));
    expect(signIn / panel).toBeGreaterThan((152 / 122) ** 2 * 0.9);
  });
});
