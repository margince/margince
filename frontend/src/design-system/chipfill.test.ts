// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { chipInks } from "../../scripts/lib/chip-inks";
import {
  classesOf,
  compounds,
  inks,
  rules,
  subjectClasses,
} from "../../scripts/lib/css-rules";
import {
  landsOn,
  overriddenInside,
  renderedInside,
} from "../../scripts/lib/rendered-inside";

const here = dirname(fileURLToPath(import.meta.url));

// What the chip fill is actually drawn under, derived rather than listed.
//
// tokens.test.ts pairs each ink in chipInks with --bgChip and measures it, which
// is a LIST of what the tree does — and a list is a second copy of it. This
// derives the corpus instead: every rule under src/ that paints
// --bgChip, plus every rule that draws INSIDE one — the same element in another
// state, or a descendant — and the ink each sets. An unmeasured ink is the
// failure nothing else would see: it reads as an ordinary declaration and no
// pair anywhere says what it comes out at over the fill.
//
// A rule is in scope when ANY compound of its selector carries every class the
// chip's SUBJECT does: `.segmented` paints a track with no ink, and `.segmented
// button` sets one that renders ON it. The subject, because `.palette-row
// .type` paints the chip and not the row. A class name is a token, not a
// prefix, so `.segmented-mark` stays out; a rule that paints a ground of its
// OWN is skipped, its ink answering to that ground.
//
// What it still cannot see, stated rather than reasoned away: an ink inherited
// from OUTSIDE the chip's subtree. A chip that declares no colour at all reads
// whatever the row around it is drawn in — `.pn-relay-owner`'s own sentence is
// one, its due date having been given an explicit ink and its lead line not —
// and no amount of reading these sheets says what that ground's ink is. The axe
// sweep over the real routes in `e2e/ac.spec.ts` is what covers that shape, and
// it is the gate that found this one.
describe("the chip fill's call sites", () => {
  // Both scans read the whole of src/, and reading it is what this file costs:
  // parsing every component to see who renders inside whom takes seconds, so
  // each check below asks for the answer rather than going and getting it
  // again. The tree does not change while the file runs.
  const sources = join(here, "..");
  const allRules = rules(sources);
  const inside = renderedInside(sources);

  // WCAG 1.4.3 exempts an inactive control, which is the whole point of the
  // dimmed tone a disabled segment takes.
  function isDisabledState(selector: string): boolean {
    return /:disabled|\[disabled\]|\[aria-disabled="true"\]/.test(selector);
  }

  function paintsOwnGround(body: string): boolean {
    return /background(?:-color)?:(?![^;]*var\(--bgChip\))[^;]*(?:var\(|#|rgb)/.test(
      body,
    );
  }

  // The containment reader, falsified. The scans below pass on this tree, and a
  // reader that had quietly stopped finding anything would leave them passing —
  // the emptiness guard catches a TOTAL failure and nothing narrower. This is
  // the case the issue was filed about, asked directly.
  it("finds the remove control inside the token that grounds it", () => {
    const belowToken = inside.get("token") ?? [];
    expect(
      belowToken.some((element) => element.has("token-remove")),
      "TokenInput renders .token-remove inside .token; the reader did not see it",
    ).toBe(true);
    // And not through a shared name: `.t-caption` renders inside the relay chip
    // and inside half the screens, so a reader that chained containment over
    // class names would put every caption inside every chip.
    const belowRelay = inside.get("pn-relay-owner") ?? [];
    expect(
      belowRelay.some((element) => element.has("compose-need")),
      "containment chained across components, which makes the scan report rules against chips they never land on",
    ).toBe(false);
  });

  it("draws on --bgChip only in inks the contrast gate measures", () => {
    const chips = allRules.filter(({ body }) =>
      /background(?:-color)?:[^;]*var\(--bgChip\)/.test(body),
    );
    // A scan that matched nothing would report PASS on an empty corpus.
    expect(chips.length).toBeGreaterThan(0);
    // An empty containment map is the way this check fails SHORT: every
    // descendant reached by its own class drops out of the subtree, and the
    // scan reports a clean tree because it stopped looking rather than because
    // there was nothing to find.
    expect(inside.size).toBeGreaterThan(0);

    const offenders: string[] = [];
    for (const chip of chips) {
      const wanted = subjectClasses(chip.selector);
      // A chip whose subject carries no class of its own — `.segmented
      // button:active` — is reached through the rule that names the track, so
      // there is nothing here to search on and nothing lost by not searching.
      if (wanted.size === 0) continue;
      // Two ways a rule is inside this chip: its SELECTOR says so, or the
      // COMPONENTS do. The second is what reaches `.token-remove`, whose rule
      // names no token class and which TokenInput nonetheless renders inside
      // one.
      const below = [...wanted].flatMap((name) => inside.get(name) ?? []);
      const subtree = allRules.filter((rule) => {
        if (rule === chip) return false;
        if (
          compounds(rule.selector).some((part) =>
            [...wanted].every((name) => classesOf(part).has(name)),
          )
        ) {
          return true;
        }
        return below.some((element) => landsOn(rule, element, below));
      });
      for (const rule of [chip, ...subtree]) {
        if (isDisabledState(rule.selector)) continue;
        if (rule !== chip && paintsOwnGround(rule.body)) continue;
        // A rule that names BOTH the chip and this element sets the ink that
        // actually lands — `.pn-relay-owner .pn-relay-due`,
        // `.filterpill[aria-pressed="true"] .filterpill-count`. The generic
        // rule beneath it is what the element is drawn in ELSEWHERE, and
        // reporting that is reporting a colour no reader sees on a chip.
        if (rule !== chip && overriddenInside(allRules, chip, rule, inside))
          continue;
        for (const ink of inks(rule.body)) {
          if (chipInks.includes(ink)) continue;
          offenders.push(
            `${relative(join(here, ".."), rule.file)}: ${rule.selector} ` +
              `draws on ${chip.selector}'s --bgChip in ${ink}, ` +
              `which no pair measures`,
          );
        }
      }
    }
    expect([...new Set(offenders)].join("\n")).toBe("");
  });

  // The other way the fill goes wrong, and the one that broke the segmented
  // strip: --bgChip painted on a DESCENDANT of something already painted in it.
  // The two composite, the ground goes a step past where the ink was measured,
  // and every pair above is measuring the wrong colour — the strip's own label
  // came out under 4.5:1 on the doubled fill. A chip is one step off its host
  // by construction, so a chip on a chip is never what was meant; the
  // state that wants to look pressed takes a ground from the ladder instead.
  it("never paints --bgChip inside something already painted in it", () => {
    const chips = allRules.filter(({ body }) =>
      /background(?:-color)?:[^;]*var\(--bgChip\)/.test(body),
    );
    expect(chips.length).toBeGreaterThan(0);
    expect(inside.size).toBeGreaterThan(0);
    const offenders: string[] = [];
    for (const chip of chips) {
      const wanted = subjectClasses(chip.selector);
      if (wanted.size === 0) continue;
      const below = [...wanted].flatMap((name) => inside.get(name) ?? []);
      for (const rule of chips) {
        if (rule === chip) continue;
        // A longer chain is a DESCENDANT; the same length carrying the same
        // classes is the same element in another state, which REPLACES the
        // fill rather than stacking on it. The components answer the case the
        // chain cannot: a descendant reached by a class of its own, which is
        // the same hole the ink scan above had.
        const chained =
          compounds(rule.selector).length > compounds(chip.selector).length &&
          compounds(rule.selector).some((part) =>
            [...wanted].every((name) => classesOf(part).has(name)),
          );
        const nested =
          chained || below.some((element) => landsOn(rule, element, below));
        if (!nested) continue;
        offenders.push(
          `${relative(join(here, ".."), rule.file)}: ${rule.selector} ` +
            `paints --bgChip inside ${chip.selector}, which already does`,
        );
      }
    }
    expect([...new Set(offenders)].join("\n")).toBe("");
  });
});
