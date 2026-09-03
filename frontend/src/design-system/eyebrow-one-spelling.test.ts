import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// `.t-eyebrow` claims to be THE one spelling of uppercase micro-type, and this
// is what makes that a claim rather than a recollection.
//
// Five sheets used to carry their own copy, at 11px or 12px and 0.03em or
// 0.04em, and every copy looked right on its own — the consolidation is in
// base.css's own comment. Two more copies then appeared INSIDE the design
// system, in the sheet for the workbench chrome, because nothing failed when
// they did. Prose recounting a past consolidation does not prevent the next
// one.
//
// The rule is not "never write these properties". It is that a rule setting
// uppercase micro-type sets it by carrying the class, so a size or a tracking
// can only be changed in the one place that owns them.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const stylesheets = join(frontendRoot, "src");

// The sheet that OWNS the eyebrow. Its own declaration is the spelling every
// other rule is measured against, so it is the one file exempt from the rule.
const owner = "src/design-system/base.css";

/** Every .css file under src, by path relative to the frontend root. */
function sheets(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return entry.name === "node_modules" || entry.name === "dist"
        ? []
        : sheets(path);
    }
    return entry.name.endsWith(".css") ? [path] : [];
  });
}

/**
 * The property pairs that TOGETHER make uppercase micro-type. A rule carrying
 * both is setting the eyebrow, whatever it calls itself.
 *
 * Both, not either: `text-transform: uppercase` alone is an ordinary choice a
 * button or a table header may make, and `--fs-eyebrow` alone sizes a caption.
 * It is the pair that restates what the class owns, which is also why the check
 * cannot be a grep for one line.
 */
const eyebrowSize = "--fs-eyebrow";
const eyebrowCase = "text-transform: uppercase";

/** One CSS rule: its selector and the declarations inside its braces. */
type Rule = { selector: string; body: string; line: number };

/**
 * The rules in one sheet, read by brace-matching rather than by regex over the
 * whole file — a regex spanning `{...}` cannot tell a rule from the gap between
 * two, and would read a size in one rule and an uppercase in the next as a
 * single offence.
 *
 * At-rules (`@media`, `@supports`) are descended into rather than skipped: a
 * copy inside a breakpoint is a copy.
 */
function rulesIn(source: string): Rule[] {
  // Comments go first, with their line breaks kept so a reported line number
  // still points at the rule. A selector read with the comment above it glued
  // to its front is not the selector, and a brace inside prose is not a rule.
  const css = source.replaceAll(/\/\*[\s\S]*?\*\//g, (block) =>
    block.replaceAll(/[^\n]/g, " "),
  );
  const out: Rule[] = [];
  let depth = 0;
  let selectorStart = 0;
  const opens: { selector: string; at: number; line: number }[] = [];
  for (let i = 0; i < css.length; i++) {
    if (css[i] === "{") {
      const selector = css.slice(selectorStart, i).trim();
      opens.push({
        selector,
        at: i + 1,
        line: css.slice(0, i).split("\n").length,
      });
      depth++;
      selectorStart = i + 1;
    } else if (css[i] === "}") {
      const open = opens.pop();
      depth--;
      if (open && !open.selector.startsWith("@")) {
        out.push({
          selector: open.selector,
          body: css.slice(open.at, i),
          line: open.line,
        });
      }
      selectorStart = i + 1;
    } else if (depth === 0 && css[i] === ";") {
      selectorStart = i + 1;
    }
  }
  return out;
}

/**
 * The sheets that still restate the eyebrow, frozen at the count they carried
 * when this gate landed. Shrinking is allowed; growing is not, and a sheet that
 * reaches zero has its line deleted.
 *
 * A COUNT PER SHEET rather than a list of lines: a line number moves whenever
 * anything above it is edited, and a baseline that churns on unrelated changes
 * is one people learn to regenerate without reading.
 *
 * Why a ratchet and not a clean sweep. Twenty-eight of these are onboarding
 * screens, and converting them is markup work with a visual answer per site —
 * worth doing, and not something to hide inside a gate landing. What matters
 * now is that the number can only go down: the two copies that appeared INSIDE
 * the design system did so because nothing failed when they did.
 */
const restated: Record<string, number> = {
  "src/design-system/composed.css": 1,
  "src/design-system/margince-workbench.css": 3,
  "src/screens/backfill.css": 1,
  "src/screens/company360.css": 1,
  "src/screens/onboarding-backread.css": 1,
  "src/screens/onboarding-build-scene.css": 1,
  "src/screens/onboarding-conversation/conversation.css": 11,
  "src/screens/onboarding-gate.css": 2,
  "src/screens/onboarding-live-panel.css": 2,
  "src/screens/onboarding.css": 4,
  "src/screens/record360/spine.css": 1,
};

/** Every rule that sets uppercase micro-type itself, by sheet. */
function restatements(): Record<string, string[]> {
  const found: Record<string, string[]> = {};
  for (const path of sheets(stylesheets)) {
    const where = relative(frontendRoot, path).replaceAll("\\", "/");
    if (where === owner) {
      continue;
    }
    for (const rule of rulesIn(readFileSync(path, "utf8"))) {
      // A NESTED rule's body contains its children's, so a parent would be
      // reported for a child's declarations. Only the rule's own text counts.
      const own = rule.body.replaceAll(/\{[^{}]*\}/g, "");
      if (own.includes(eyebrowSize) && own.includes(eyebrowCase)) {
        found[where] ??= [];
        found[where].push(`${rule.selector} (line ${rule.line})`);
      }
    }
  }
  return found;
}

describe("the eyebrow has one spelling", () => {
  it("gains no new sheet that sets uppercase micro-type itself", () => {
    const found = restatements();
    const arrived = Object.keys(found)
      .filter((where) => !(where in restated))
      .sort();
    expect(
      arrived,
      "these sheets set uppercase micro-type themselves instead of carrying " +
        ".t-eyebrow, which is what let five sheets disagree about its size and " +
        "tracking. Add the class to the element and delete the properties",
    ).toEqual([]);
  });

  it("lets no ratified sheet grow another copy", () => {
    const found = restatements();
    const grew = Object.entries(restated)
      .filter(([where, frozen]) => (found[where]?.length ?? 0) > frozen)
      .map(
        ([where, frozen]) =>
          `${where}: ${found[where]?.length} now, ${frozen} when this was frozen — ` +
          `${found[where]?.join(", ")}`,
      );
    expect(grew, "the baseline only shrinks").toEqual([]);
  });

  it("reports a sheet that has been cleaned, so the baseline shrinks", () => {
    const found = restatements();
    const shrunk = Object.entries(restated)
      .filter(([where, frozen]) => (found[where]?.length ?? 0) < frozen)
      .map(
        ([where, frozen]) =>
          `${where}: ${found[where]?.length ?? 0} now, ${frozen} in the baseline`,
      );
    expect(
      shrunk,
      "these sheets carry fewer copies than the baseline records. That is the " +
        "direction this gate exists to allow — lower the number, or delete the " +
        "line when it reaches zero",
    ).toEqual([]);
  });

  // The gate must be able to SEE an offence, or a clean tree and a broken
  // reader look identical. The owner's own rule is the offence shape, so
  // finding it there proves the reader works on the real file.
  it("reads the owner's own declaration as the shape it hunts", () => {
    const css = readFileSync(join(frontendRoot, owner), "utf8");
    const eyebrow = rulesIn(css).find((rule) => rule.selector === ".t-eyebrow");
    expect(eyebrow, ".t-eyebrow is gone from base.css").toBeDefined();
    expect(eyebrow?.body).toContain(eyebrowSize);
    expect(eyebrow?.body).toContain(eyebrowCase);
  });
});
