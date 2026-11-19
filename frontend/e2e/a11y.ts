// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import AxeBuilder from "@axe-core/playwright";
import type { Page } from "@playwright/test";

/**
 * The AA sweep of one page: assert what axe DECIDED, report what it could not.
 *
 * `violations` is the gate, and it stays the gate. `incomplete` is deliberately
 * reported rather than asserted, and the reason is what the word means: axe
 * puts a node there when it cannot reach a verdict at all — a colour over a
 * gradient or an image, a pseudo-element, or (the case in this tree) a `.cf-count`
 * whose "content [is] too short to determine if it is actual text content".
 * None of those become decidable by fixing the page, so a blanket
 * `expect(incomplete).toEqual([])` could only be satisfied by deleting the
 * assertion again — and a gate that gets weakened once teaches everyone that
 * gates get weakened.
 *
 * So the undecided findings are PRINTED, with axe's own reason attached, on a
 * line a human reading the run can see. That is a real gap and it is stated as
 * one: an incomplete `color-contrast` is a colour nobody has verified, and the
 * only thing standing behind those today is the token law in tokens.css — there
 * are two neutral inks, `--textPrimary` and `--textSecondary`, and nothing
 * quieter than the second.
 */
// What a colour-contrast check carries when it could MEASURE the pair. Axe
// types `data` as unknown and fills it per check, so a rule that is not
// colour-contrast carries something else entirely and colour-contrast itself
// omits these when it could not resolve a background — the "partially overlaps
// other elements" findings are exactly that shape.
//
// All four are required together rather than each defaulting, because a
// half-filled reading is what this reporting exists to prevent: "#5e6c65 on
// undefined = undefined:1" reads like a measurement and is not one, and it
// would be believed. A check that cannot fill them falls back to axe's own
// sentence, which at least says why.
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

// HEADING_RULES are run BESIDE the WCAG tags rather than inside them, because
// axe tags both `best-practice` only. Neither carries a WCAG tag, so the tag
// list below ran neither on any route: the sweep read a smaller rule set than
// "the axe sweep is green" implies, with no failing assertion to say so.
//
// By id rather than by adding the `best-practice` tag, which would widen the
// sweep well past headings in one step. These two are the measured gap;
// adopting the rest of best-practice is a separate decision with its own tail.
const HEADING_RULES = ["heading-order", "page-has-heading-one"];

// TWO ANALYSES, and it has to be two.
//
// `AxeBuilder.withRules` does not ADD to `withTags` — it sets
// `runOnly: {type: "rule"}`, replacing the tag selection outright, and the
// library's own doc says the two cannot be combined. Chaining them narrows the
// sweep from every WCAG rule to these two, which is a worse version of the
// defect this exists to fix and would have reported PASS the whole way.
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
  // A violation names the SELECTOR and nothing else, which is unactionable for
  // the one class of failure this sweep actually produces: a contrast finding
  // that appears on CI and on no developer's machine. Four runs named four
  // different screens and every finding was small meta text, and none of them
  // said what colours were measured — so there was nothing to compare against
  // the tokens, and the investigation ran on hypotheses instead of readings.
  //
  // Axe already computed the pair. Reporting it costs nothing on a green run
  // and turns a red one into evidence: the resolved theme, the two colours and
  // the ratio, beside the threshold that rejected them.
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
    // The document's own theme, not the emulated scheme: a stamped
    // `data-theme` outranks `prefers-color-scheme`, so the scheme a test asked
    // for is not proof of the palette it got.
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
