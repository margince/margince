// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import AxeBuilder from "@axe-core/playwright";
import { expect, type Page } from "@playwright/test";

/**
 * Settle the page's motion before measuring the colours it paints.
 *
 * A contrast check reads the painted colours, and an element mid-fade paints a
 * blend. Axe would then measure a ratio no reader sees, and the result would
 * depend on the machine's speed.
 *
 * Waiting for `getAnimations()` to report every finite animation finished
 * cannot settle a page. That set is empty both before the first entrance starts
 * and after the last one ends. `[].every()` is true of an empty set, so the
 * wait can return before content arrives. `.wrap > *` holds each block back by
 * up to five steps of `--stagger-enter` before it fades in.
 *
 * Reduced motion settles it instead. The design system answers it with
 * `animation: none` on every arrival. Content already mounted and content still
 * to mount both land at their resting colour. The assertions below cannot pass
 * vacuously: an arrival that outlives the guard fails here and names itself.
 */
export async function settleAnimations(page: Page) {
  await page.emulateMedia({ reducedMotion: "reduce" });
  // Reduced motion stops the next animation, not one already in flight. The
  // design system's Select opens through the Web Animations API. It checks
  // usePrefersReducedMotion() at render time. A test that clicks first and
  // emulates after has started an animation the media query cannot stop.
  // CSS `animation: none` does not govern a WAAPI animation either.
  //
  // So each in-flight animation is awaited to its resting frame. Only finite
  // animations are awaited, judged by their own timing and not by a clock. A
  // perpetual one never resolves `finished`, so the check below names it.
  // A wall-clock budget cannot tell "perpetual" from "slow" on a loaded runner.
  //
  // allSettled rather than all: a cancelled animation rejects `finished`, and
  // a cancelled animation is over, which is all this waits for.
  //
  // One look is not enough: a boot transition can start after it. So the wait
  // repeats until a pass finds no finite clock-driven animation running. The
  // round count is a backstop against a page that keeps starting new ones.
  // Hitting it names them rather than handing them on as a finding.
  const unsettled = await page.evaluate(async () => {
    const lands = (animation: Animation) =>
      animation.timeline instanceof DocumentTimeline &&
      Number.isFinite(
        animation.effect?.getComputedTiming().activeDuration ?? Infinity,
      );
    const landing = () =>
      document
        .getAnimations()
        .filter((animation) => animation.playState === "running")
        .filter(lands);
    for (let round = 0; round < 50; round++) {
      const running = landing();
      if (running.length === 0) {
        return [];
      }
      await Promise.allSettled(running.map((animation) => animation.finished));
    }
    return landing().map((animation) => {
      const target =
        animation.effect instanceof KeyframeEffect
          ? animation.effect.target
          : null;
      return target
        ? `${target.tagName.toLowerCase()}.${target.className}`
        : "?";
    });
  });
  expect(
    unsettled,
    "finite animations kept starting through 50 rounds of waiting for them to land",
  ).toEqual([]);
  const motion = await page.evaluate(() => {
    const describe = (element: Element) =>
      `${element.tagName.toLowerCase()}.${element.className}`;
    return {
      running: document
        .getAnimations()
        .filter((animation) => animation.playState === "running")
        // A scroll-driven animation follows the scroll position, not the
        // clock, so axe reads the frame a reader at that position sees.
        .filter((animation) => animation.timeline instanceof DocumentTimeline)
        .map((animation) => {
          const effect = animation.effect;
          return effect instanceof KeyframeEffect &&
            effect.target instanceof Element
            ? describe(effect.target)
            : "an element the effect does not name";
        }),
      // The guard itself, read off the cascade rather than off the clock. An
      // arrival that still names an animation here fades whenever it mounts,
      // even after this check. Only the computed style shows that.
      stillArriving: [...document.querySelectorAll(".arrive, .wrap > *")]
        .filter((element) => getComputedStyle(element).animationName !== "none")
        .map(describe),
    };
  });
  expect(
    motion.stillArriving,
    "an arrival outlived the reduced-motion guard in enter.css, so content fades in whenever it mounts and axe reads the blend rather than the colours a reader sees",
  ).toEqual([]);
  expect(
    motion.running,
    "an animation kept running under prefers-reduced-motion, so axe would read a frame mid-flight",
  ).toEqual([]);
}

/**
 * The AA sweep of one page: assert what axe decided, report what it could not.
 *
 * `violations` is the gate. `incomplete` is reported rather than asserted. Axe
 * puts a node there when it cannot reach a verdict at all. Examples are a
 * colour over a gradient or image, a pseudo-element, or a `.cf-count` too
 * short to count as text. Fixing the page cannot make those decidable, so
 * asserting `incomplete` empty would fail for good.
 *
 * So the undecided findings are printed with axe's own reason, where a human
 * reading the run can see them. An incomplete `color-contrast` is a colour
 * nobody has verified. The token rule in tokens.css covers those today. It has
 * two neutral inks, `--textPrimary` and `--textSecondary`, and nothing quieter.
 */
// What a colour-contrast check carries when it could measure the pair. Axe
// types `data` as unknown and fills it per check. Other rules carry something
// else. Colour-contrast omits these when it could not resolve a background,
// as in the "partially overlaps other elements" findings.
//
// All four are required together, with no defaults. A half-filled reading such
// as "#5e6c65 on undefined = undefined:1" looks like a measurement and is not
// one. A check that cannot fill them falls back to axe's own sentence.
type ContrastReading = Readonly<{
  fgColor: string;
  bgColor: string;
  contrastRatio: number;
  expectedContrastRatio: string;
}>;

function isContrastReading(data: unknown): data is ContrastReading {
  if (typeof data !== "object" || data === null) {
    return false;
  }
  return (
    "fgColor" in data &&
    typeof data.fgColor === "string" &&
    "bgColor" in data &&
    typeof data.bgColor === "string" &&
    "contrastRatio" in data &&
    typeof data.contrastRatio === "number" &&
    "expectedContrastRatio" in data &&
    typeof data.expectedContrastRatio === "string"
  );
}

// HEADING_RULES run beside the WCAG tags rather than inside them. Axe tags
// both rules `best-practice` only, so the WCAG tag list never runs them.
//
// They are named by id rather than by the `best-practice` tag. That tag would
// widen the sweep well past headings, which is a separate decision.
const HEADING_RULES = ["heading-order", "page-has-heading-one"];

// Two analyses, and it has to be two.
//
// `AxeBuilder.withRules` does not add to `withTags`. It sets
// `runOnly: {type: "rule"}` and replaces the tag selection. The library's own
// doc says the two cannot be combined. Chaining them would narrow the sweep
// from every WCAG rule to these two, and it would still pass.
//
// So the tags run, the rules run, and the violations are read together.
async function axeFindings(page: Page) {
  const wcag = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
    .analyze();
  const headings = await new AxeBuilder({ page })
    .withRules(HEADING_RULES)
    .analyze();
  return {
    violations: [...wcag.violations, ...headings.violations],
    incomplete: [...wcag.incomplete, ...headings.incomplete],
  };
}

export async function expectNoAaViolations(page: Page, screen: string) {
  const results = await axeFindings(page);
  const undecided = results.incomplete.flatMap((check) =>
    check.nodes.map(
      (node) =>
        `${check.id}: ${node.target.join(" ")} — ${node.any
          .map((result) => result.message)
          .join("; ")}`,
    ),
  );
  if (undecided.length > 0) {
    console.warn(
      `axe could not decide ${undecided.length} finding(s) on #/${screen}; a human has to:\n  ${undecided.join("\n  ")}`,
    );
  }
  // A violation names only the selector. That cannot explain a contrast
  // finding that shows on CI and on no developer's machine.
  //
  // Axe already computed the pair. Reporting it costs nothing on a green run.
  // A red run then shows the resolved theme, the two colours and the ratio,
  // beside the threshold that rejected them.
  const failures = results.violations.flatMap((violation) =>
    violation.nodes.map((node) => {
      const detail = node.any
        .map((check) =>
          isContrastReading(check.data)
            ? `${check.data.fgColor} on ${check.data.bgColor} = ${check.data.contrastRatio}:1, needs ${check.data.expectedContrastRatio}`
            : check.message,
        )
        .join("; ");
      return `${violation.id}: ${node.target.join(" ")} — ${detail}`;
    }),
  );
  if (failures.length > 0) {
    // The document's own theme, not the emulated scheme. A stamped
    // `data-theme` outranks `prefers-color-scheme`, so the requested scheme
    // does not prove the palette.
    const painted = await page.evaluate(() => ({
      dataTheme: document.documentElement.getAttribute("data-theme"),
      prefersDark: window.matchMedia("(prefers-color-scheme: dark)").matches,
      textSecondary: getComputedStyle(document.documentElement)
        .getPropertyValue("--textSecondary")
        .trim(),
      bgPage: getComputedStyle(document.documentElement)
        .getPropertyValue("--bgPage")
        .trim(),
    }));
    throw new Error(
      `axe found ${failures.length} AA violation(s) on #/${screen}\n` +
        `  painted as: ${JSON.stringify(painted)}\n  ` +
        failures.join("\n  "),
    );
  }
}
