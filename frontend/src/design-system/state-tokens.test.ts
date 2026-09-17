// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  blocks,
  linearOf,
  linearToOklab,
  normalize,
  parseBlock,
  states,
  themes,
  tokenDecls,
} from "./tokens-testing";

// THE FIVE STATES ARE A FAMILY, AND EVERY MEMBER DERIVES FROM ITS BASE.
//
// Its own file, beside tokens.test.ts rather than inside it: that gate asks
// whether the palette holds its canonical VALUES and whether the surface ladder
// holds its ORDER, and this one asks whether one base still governs the four
// tokens drawn from it. They share the sheet and the colour maths in
// tokens-testing.ts and nothing else, and the state list every sweep here walks
// is read off the sheet rather than written down — a hand-kept list is the one
// failure that reports PASS over a family it stopped covering.

describe("the state families", () => {
  // The one place the five names are written. It fails in BOTH directions on
  // purpose: a state dropped from tokens.css takes its sweeps with it in
  // silence, and a sixth one arriving is a palette decision somebody has to
  // make rather than a family these gates quietly start covering.
  it("names five states and no others", () => {
    expect(states).toEqual([
      "info",
      "success",
      "warning",
      "danger",
      "discovery",
    ]);
  });
  // A state's tint and its opaque Surface are one share spelled twice, so
  // the Surface must read as the tint with the elevated ground standing in
  // for transparent. Which arm a member takes is decided by the FORM it is
  // written in and not by which state it belongs to: a value nobody
  // recognises throws rather than passing unread, so a sixth spelling cannot
  // arrive as a silent exemption.
  it("derives each state's tints from its own base, in every theme", () => {
    // The share a member takes of its own base, or undefined when the member
    // is not written as one.
    const shareOfBase = (value: string, state: string) =>
      value.match(
        new RegExp(
          `^color-mix\\(\\s*in srgb\\s*,\\s*var\\(--${state}\\)\\s*(\\d+)%\\s*,`,
        ),
      )?.[1];
    for (const [theme, block] of Object.entries(blocks)) {
      for (const state of states) {
        const tint = shareOfBase(block[`--${state}Bg`], state);
        expect(
          tint,
          `${theme}: --${state}Bg is not a share of --${state}`,
        ).toBeDefined();

        for (const member of ["Surface", "Border"] as const) {
          const value = block[`--${state}${member}`];
          const share = shareOfBase(value, state);
          if (share === undefined) {
            // A stated hex is the other lawful spelling — discovery's light
            // ramp is given directly by the design source. It buys no
            // exemption: the hue assertion below measures exactly these.
            expect(
              value.trim(),
              `${theme}: --${state}${member} is neither a share of ` +
                `--${state} nor a stated hex — nothing can measure it`,
            ).toMatch(/^#[0-9a-f]{6}$/i);
            continue;
          }
          if (member === "Border") {
            expect(
              share,
              `${theme}: --${state}Border is not the family's 45% hairline`,
            ).toBe("45");
            continue;
          }
          // The opaque tint and the translucent one are ONE share written
          // twice, so the Surface must read as the Bg with the elevated
          // ground standing in for transparent.
          expect(
            normalize(value),
            `${theme}: --${state}Surface and --${state}Bg state two shares`,
          ).toBe(
            normalize(
              block[`--${state}Bg`].replace("transparent", "var(--bgElevated)"),
            ),
          );
        }
      }
    }
  });

  // Every member of a family is the SAME COLOUR at another lightness, which
  // is the claim that makes one base enough. Held in OKLCh, where a hue is a
  // number: a walk down in lightness keeps it, and a value picked by eye and
  // pasted in does not. The stated hexes — discovery's light ramp — are
  // measured by exactly this, so being written out costs them no rigour.
  //
  // The Bg and Surface tints are excluded on purpose and not by oversight: a
  // tint is the hue MIXED WITH THE GROUND, and this tree's grounds are
  // themselves faintly green, so a correct tint lands up to 20° off its base.
  // The share rule above is what holds those.
  it("keeps every member of a state on its base's own hue", () => {
    const hueOf = (value: string) => {
      const [, a, b] = linearToOklab(linearOf(value));
      return ((Math.atan2(b, a) * 180) / Math.PI + 360) % 360;
    };
    const apart = (one: number, other: number) => {
      const gap = Math.abs(one - other) % 360;
      return gap > 180 ? 360 - gap : gap;
    };
    const failures: string[] = [];
    for (const [theme, pal] of Object.entries(themes)) {
      for (const state of states) {
        const base = hueOf(pal[`--${state}`]);
        for (const member of ["Text", "Border"]) {
          const value = pal[`--${state}${member}`];
          // A Border is a share of the base over transparent and keeps the
          // hue exactly; an ink is the base walked in lightness. Two degrees
          // is the room 8-bit rounding needs and no more.
          if (!value.startsWith("#")) continue;
          const drift = apart(base, hueOf(value));
          if (drift > 2) {
            failures.push(
              `${theme}: --${state}${member} (${value}) sits ` +
                `${drift.toFixed(1)}° off --${state} (${pal[`--${state}`]})`,
            );
          }
        }
      }
    }
    expect(failures.join("\n")).toBe("");
  });

  // Fail closed on a missing state. A theme that states no base for one of
  // them does not render it wrong, it renders it in the OTHER theme's colour
  // — and every sweep above still passes, because each reads whatever the
  // light block left behind. So both dark arms are asked for all five by
  // name, including the two whose value does not change.
  it("states every state's base and ink in both dark arms", () => {
    for (const selector of [
      '[data-theme="dark"]',
      ':root:not([data-theme="light"])',
    ]) {
      const block = parseBlock(tokenDecls, selector);
      for (const state of states) {
        expect(
          block[`--${state}`],
          `${selector} states no --${state}`,
        ).toBeDefined();
        expect(
          block[`--${state}Text`],
          `${selector} states no --${state}Text`,
        ).toBeDefined();
      }
    }
  });
});
