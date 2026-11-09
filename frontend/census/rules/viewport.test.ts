// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import {
  FOLD_MAX_WIDTH,
  NARROW_MAX_WIDTH,
  PHONE_MAX_WIDTH,
} from "../../src/lib/viewport";
import { plantedFindings, srcDir } from "../tree";
import {
  mediaWidths,
  misspelling,
  namedWidths,
  rule,
  UNNAMED_WIDTHS,
} from "./viewport.census";

describe("the viewport rule", () => {
  it("reads every upper width a media query can spell", () => {
    const read = (css: string) =>
      mediaWidths(css).map(({ px, exclusive }) => ({ px, exclusive }));
    const upTo = (px: number) => [{ px, exclusive: false }];
    const below = (px: number) => [{ px, exclusive: true }];
    expect(read("@media (max-width: 700px) { a {} }")).toEqual(upTo(700));
    expect(read("@media (width <= 699px) { a {} }")).toEqual(upTo(699));
    expect(read("@media (width < 40rem) { a {} }")).toEqual(below(640));
    expect(read("@media (720px >= width) { a {} }")).toEqual(upTo(720));
    expect(read("@media (45em > width) { a {} }")).toEqual(below(720));
    expect(read("@media (pointer: coarse) and (max-width: 701px) {")).toEqual(
      upTo(701),
    );
    expect(read("@media (600px <= width <= 1100px) { a {} }")).toEqual(
      upTo(1100),
    );
    expect(read("/* @media (max-width: 690px) { */ a {}")).toEqual([]);
    expect(read("@media (min-width: 700px) { a {} }")).toEqual([]);
    expect(read("@media (width >= 700px) { a {} }")).toEqual([]);
    expect(read("@container (max-width: 700px) { a {} }")).toEqual([]);
  });

  it("accepts a named or listed width, and fails a drift at any distance", () => {
    const names = namedWidths();
    expect(names.map(({ name }) => name)).toEqual(
      expect.arrayContaining([
        "PHONE_MAX_WIDTH",
        "FOLD_MAX_WIDTH",
        "NARROW_MAX_WIDTH",
      ]),
    );
    const upTo = (px: number) =>
      misspelling({ prelude: "", px, exclusive: false });
    for (const { px } of names) {
      expect(upTo(px)).toBeUndefined();
      expect(UNNAMED_WIDTHS).not.toContain(px);
    }
    expect(upTo(760)).toBeUndefined();
    expect(upTo(FOLD_MAX_WIDTH + 20)).toMatch(/not FOLD_MAX_WIDTH \(720px\)/);
    expect(upTo(PHONE_MAX_WIDTH - 1)).toMatch(/not PHONE_MAX_WIDTH/);
    expect(upTo(NARROW_MAX_WIDTH + 60)).toMatch(/not NARROW_MAX_WIDTH/);
    expect(
      misspelling({ prelude: "", px: FOLD_MAX_WIDTH, exclusive: true }),
    ).toMatch(/strict "<"/);
  });

  it("fails a planted sheet that drifts, and a listed width nobody spells", () => {
    const findings = plantedFindings(rule, [
      {
        path: join(srcDir, "features", "x", "x.css"),
        area: "src",
        text: `@media (max-width: ${FOLD_MAX_WIDTH + 1}px) { a { b: c } }`,
      },
    ]);
    expect(findings[0]).toMatch(
      /^src\/features\/x\/x\.css: @media \(max-width: 721px\) spells 721px/,
    );
    expect(findings).toHaveLength(1 + UNNAMED_WIDTHS.length);
  });
});
