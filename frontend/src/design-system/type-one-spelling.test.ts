// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { extensionLayers, filesMatching } from "../../scripts/lib/source-tree";

// A type utility has one spelling, and it is the class.
//
// `base.css` declares the type utilities — `.t-caption` is quiet grey at the
// meta size, `.t-eyebrow` uppercase micro-type, `.t-h3` a card title — and a
// screen that wants one carries the class. What the tree did instead was spell
// the utility's properties again under a name of its own: 135 screen rules
// carried `font-size: var(--fs-meta); color: var(--textMeta)` verbatim, each
// one a `.t-caption` that stops moving when `.t-caption` does. Every one of
// them read a token, so `type.test.ts` — which holds the VOCABULARY, every size
// a rung — passed all of them. This gate holds the GRAMMAR: a rule that says
// everything a utility says is that utility, and carries its class.
//
// Two arms, because two shapes of the offence:
//
//   restatement — a rule's own declarations include EVERY declaration of some
//                 utility, at the same value. The rule may say more (a margin,
//                 a max-width — those are its own), and it keeps those; the
//                 type properties move to the class on the element. Held at
//                 zero: the tree was swept before this arm was armed.
//
//                 Screens and app chrome only. `design-system/` DEFINES the
//                 primitives, and a primitive whose type equals a utility's — a
//                 badge at meta grey, a tab label at the dense size — is that
//                 primitive's own definition, not a caption under the wrong
//                 name. A reader of one rule cannot tell that coincidence from a
//                 copy, and in the tier that owns both, a wrong fire teaches
//                 people to waive. The document root is the other exemption:
//                 `body` setting the body size is the default being declared,
//                 not a rule restating it.
//
//   eyebrow     — a rule that sets `text-transform: uppercase` at micro-type
//                 size is drawing the eyebrow whatever else it says, because
//                 uppercase micro-type IS the eyebrow and nothing else in the
//                 design language is set that way. Looser than the first arm on
//                 purpose: the copies differ in colour, weight and tracking,
//                 which is exactly the drift the class exists to end. Held as
//                 a RATCHET — a per-sheet count that may only fall — because
//                 converting one is markup with a visual answer per site.
//
// Both arms READ their subject from base.css. A gate that restated the
// utilities would be one more copy of them, which is the defect it exists to
// stop; what it asserts about the owner is that the utilities are still there
// to be read, or a clean tree and a broken reader would look the same.
//
// The waiver is `ds:ignore <reason>` in a comment inside the rule, the spelling
// the other design-system gates already taught the tree. A marker with no
// reason is itself a finding.
//
// Under-recognition is the way this gate must not break: a corpus that misses
// files, a reader that skips a breakpoint, a signature that stops matching.
// So the corpus fails closed when it is small, at-rules are descended into,
// and the detector carries planted cases of every shape it has to see.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

/** The sheet that OWNS the utilities. Its declarations are the spelling every
 *  other rule is measured against, so it is the one file exempt from both arms. */
const owner = "src/design-system/base.css";

/** The tier that defines the primitives; see the restatement arm above. */
const definingTier = "src/design-system/";

/** The document itself, whose type IS the default the utilities reset to. */
const documentRoot = /^(html|body|:root)$/;

/**
 * A document that does not load the class layer cannot carry the class. Each
 * MCP-app view is served standalone to a third-party host and imports
 * `tokens.css` and nothing else — so a rule there spelling meta grey by hand is
 * the only way it can be spelled. check-ds-spacing-roles.sh draws the same line
 * for the same reason.
 */
const standaloneDocuments = "src/mcp-apps/";

/** Every stylesheet the gate reads, by path relative to the frontend root. */
function corpus(): string[] {
  const core = filesMatching(join(frontendRoot, "src"), /\.css$/);
  // A unit's screen ships into the same page as core and is held to the same
  // utilities. Presence under extensions/ is the enablement.
  const units = extensionLayers(join(frontendRoot, "..", "extensions")).flatMap(
    (layer) => filesMatching(layer, /\.css$/),
  );
  return core
    .concat(units)
    .map((path) => relative(frontendRoot, path).replaceAll("\\", "/"))
    .filter(
      (where) => where !== owner && !where.startsWith(standaloneDocuments),
    );
}

/** One CSS rule: its selector, its OWN declarations, and where it starts. */
type Rule = { selector: string; body: string; raw: string; line: number };

/**
 * The rules in one sheet, read by brace-matching rather than by regex over the
 * whole file — a regex spanning `{...}` cannot tell a rule from the gap between
 * two. At-rules (`@media`, `@supports`) are descended into rather than
 * skipped: a copy inside a breakpoint is a copy. A nested rule's declarations
 * belong to the nested rule, not to its parent.
 *
 * `body` has comments blanked (line breaks kept, so a reported line still
 * points at the rule); `raw` keeps them, because the waiver lives in one.
 */
function rulesIn(source: string): Rule[] {
  const css = source.replaceAll(/\/\*[\s\S]*?\*\//g, (block) =>
    block.replaceAll(/[^\n]/g, " "),
  );
  const out: Rule[] = [];
  let selectorStart = 0;
  const opens: { selector: string; at: number; line: number }[] = [];
  for (let i = 0; i < css.length; i++) {
    if (css[i] === "{") {
      opens.push({
        selector: css.slice(selectorStart, i).trim(),
        at: i + 1,
        line: css.slice(0, i).split("\n").length,
      });
      selectorStart = i + 1;
    } else if (css[i] === "}") {
      const open = opens.pop();
      if (open && !open.selector.startsWith("@")) {
        out.push({
          selector: open.selector,
          body: css.slice(open.at, i).replaceAll(/\{[^{}]*\}/g, ""),
          raw: source.slice(open.at, i),
          line: open.line,
        });
      }
      selectorStart = i + 1;
    } else if (css[i] === ";") {
      // Inside a rule this ends a declaration, and what follows up to the next
      // `{` is a NESTED rule's selector; at the top it ends an at-rule.
      selectorStart = i + 1;
    }
  }
  return out;
}

/** A rule's declarations as property → value, whitespace normalised. */
function declarations(body: string): Map<string, string> {
  const out = new Map<string, string>();
  for (const declaration of body.split(";")) {
    const colon = declaration.indexOf(":");
    if (colon < 0) {
      continue;
    }
    out.set(
      declaration.slice(0, colon).trim().toLowerCase(),
      declaration
        .slice(colon + 1)
        .trim()
        .replaceAll(/\s+/g, " "),
    );
  }
  return out;
}

type Utility = { name: string; signature: Map<string, string> };

/**
 * The type utilities, as base.css declares them: every `.t-*` rule that sets a
 * size. Most specific first, so a rule spelling `.t-label` (size, weight,
 * colour) is reported as `.t-label` and not as the `.t-caption` inside it.
 */
function utilities(ownerCss: string): Utility[] {
  return rulesIn(ownerCss)
    .filter((rule) => /^\.t-[a-z0-9-]+$/.test(rule.selector))
    .map((rule) => ({
      name: rule.selector,
      signature: declarations(rule.body),
    }))
    .filter((utility) => utility.signature.has("font-size"))
    .sort((a, b) => b.signature.size - a.signature.size);
}

/** The utility a rule restates, if its own declarations say everything one says. */
function restates(rule: Rule, known: Utility[]): Utility | undefined {
  const own = declarations(rule.body);
  return known.find((utility) =>
    [...utility.signature].every(
      ([property, value]) => own.get(property) === value,
    ),
  );
}

/**
 * The eyebrow's own pair, read from its rule: the case it is set in, and the
 * sizes that are micro-type. A rule carrying both is setting the eyebrow,
 * whatever it calls itself and whatever else it says.
 *
 * Both, not either: `text-transform: uppercase` alone is an ordinary choice a
 * button or a table header may make, and a small size alone sizes a monogram.
 * Micro-type is the eyebrow's own rung and the one above it: `--fs-meta` is
 * the top of it, because uppercase at `--fs-sm` or above is a heading or a
 * button rather than a label drawn by hand. Only tokens are read, because
 * `type.test.ts` holds every hand-typed size at zero — a copy spelling `11px`
 * cannot exist to miss.
 */
type EyebrowPair = { sizes: Set<string>; case: string };

function eyebrowPair(known: Utility[]): EyebrowPair {
  const eyebrow = known.find((utility) => utility.name === ".t-eyebrow");
  if (!eyebrow) {
    throw new Error(".t-eyebrow is gone from base.css");
  }
  const size = eyebrow.signature.get("font-size");
  const textCase = eyebrow.signature.get("text-transform");
  if (!size || !textCase) {
    throw new Error(".t-eyebrow no longer declares a size and a case");
  }
  return { sizes: new Set([size, "var(--fs-meta)"]), case: textCase };
}

function drawsTheEyebrow(rule: Rule, pair: EyebrowPair): boolean {
  const own = declarations(rule.body);
  const size = own.get("font-size");
  return (
    size !== undefined &&
    pair.sizes.has(size) &&
    own.get("text-transform") === pair.case
  );
}

/**
 * A waiver is the sentence, not the marker: `ds:ignore` alone says a rule was
 * skipped and not why, which is the note the next reader cannot act on.
 *
 * It sits on the `font-size` declaration, or on the line above it — where the
 * other design-system gates put theirs, and the one place that cannot be read
 * as waiving something else. A rule often carries a spacing waiver of its own
 * on a margin, and a gate that read any marker in the rule as its own would be
 * disarmed by it.
 */
type Waiver = "none" | "reasoned" | "bare";

function waiverIn(rule: Rule): Waiver {
  const lines = rule.raw.split("\n");
  const sized = lines.findIndex((line) => /^\s*font-size\s*:/.test(line));
  if (sized < 0) {
    return "none";
  }
  const nearby = `${lines[sized - 1] ?? ""}\n${lines[sized]}`;
  const marker = /ds:ignore\b([^*\n]*)/.exec(nearby);
  if (!marker) {
    return "none";
  }
  return marker[1].trim().length > 0 ? "reasoned" : "bare";
}

type Finding = { where: string; rule: Rule; says: string };

/** Both arms over the whole corpus, waivers applied. */
function findings(): {
  restated: Finding[];
  eyebrows: Finding[];
  bare: Finding[];
} {
  const known = utilities(readFileSync(join(frontendRoot, owner), "utf8"));
  const pair = eyebrowPair(known);
  const restated: Finding[] = [];
  const eyebrows: Finding[] = [];
  const bare: Finding[] = [];
  for (const where of corpus()) {
    const defines = where.startsWith(definingTier);
    for (const rule of rulesIn(
      readFileSync(join(frontendRoot, where), "utf8"),
    )) {
      const eyebrow = drawsTheEyebrow(rule, pair);
      // A rule drawing the eyebrow also says what `.t-caption` says, and it is
      // the eyebrow it is restating — so it goes to the ratchet unless it is
      // the eyebrow spelled in full, which the first arm holds at zero.
      const spelled =
        defines || documentRoot.test(rule.selector)
          ? undefined
          : restates(rule, known);
      const utility =
        spelled && (!eyebrow || spelled.name === ".t-eyebrow")
          ? spelled
          : undefined;
      if (!utility && !eyebrow) {
        continue;
      }
      const waiver = waiverIn(rule);
      if (waiver === "bare") {
        bare.push({ where, rule, says: "ds:ignore with no reason" });
      }
      if (waiver !== "none") {
        continue;
      }
      if (utility) {
        restated.push({ where, rule, says: utility.name });
      } else {
        eyebrows.push({ where, rule, says: ".t-eyebrow" });
      }
    }
  }
  return { restated, eyebrows, bare };
}

function shown(finding: Finding): string {
  return `${finding.where}:${finding.rule.line}  ${finding.rule.selector}  →  ${finding.says}`;
}

/**
 * The sheets that still draw the eyebrow by hand, frozen at the count they
 * carried when the ratchet was set. Shrinking is allowed; growing is not, and a
 * sheet that reaches zero has its line deleted.
 *
 * A COUNT PER SHEET rather than a list of lines: a line number moves whenever
 * anything above it is edited, and a baseline that churns on unrelated changes
 * is one people learn to regenerate without reading.
 */
const eyebrowBaseline: Record<string, number> = {
  "src/app/agentrail.css": 1,
  "src/app/shell.css": 1,
  "src/design-system/atoms.css": 1,
  "src/design-system/composed.css": 4,
  "src/design-system/listtable.css": 2,
  "src/design-system/margince-workbench.css": 3,
  "src/screens/auth.css": 1,
  "src/screens/backfill.css": 1,
  "src/screens/company360.css": 2,
  "src/screens/onboarding-backread.css": 1,
  "src/screens/onboarding-conversation/conversation.css": 2,
  "src/screens/onboarding-gate.css": 2,
  "src/screens/onboarding-live-panel.css": 2,
  "src/screens/onboarding.css": 5,
  "src/screens/person360.css": 2,
  "src/screens/preferences.css": 1,
  "src/screens/record360/spine.css": 1,
};

describe("a type utility has one spelling", () => {
  const found = findings();

  it("reads a corpus and an owner that are not empty", () => {
    expect(corpus().length).toBeGreaterThan(100);
    const known = utilities(readFileSync(join(frontendRoot, owner), "utf8"));
    expect(known.map((utility) => utility.name)).toContain(".t-caption");
    expect(known.map((utility) => utility.name)).toContain(".t-eyebrow");
    expect(known.length).toBeGreaterThanOrEqual(6);
  });

  it("finds no rule that spells a utility instead of carrying it", () => {
    expect(
      found.restated.map(shown),
      "each of these rules says everything a type utility says. Put the class " +
        "on the element and delete those properties from the rule — what the " +
        "rule says beyond the utility (a margin, a width) stays. A rule that " +
        "must differ from the utility says so in the rule: /* ds:ignore <reason> */\n",
    ).toEqual([]);
  });

  it("finds no waiver without a reason", () => {
    expect(
      found.bare.map(shown),
      "ds:ignore says a rule was skipped and not why; write the reason after it",
    ).toEqual([]);
  });

  describe("the eyebrow ratchet", () => {
    const bySheet: Record<string, Finding[]> = {};
    for (const finding of found.eyebrows) {
      bySheet[finding.where] ??= [];
      bySheet[finding.where].push(finding);
    }

    it("gains no new sheet that draws uppercase micro-type itself", () => {
      const arrived = Object.keys(bySheet)
        .filter((where) => !(where in eyebrowBaseline))
        .flatMap((where) => bySheet[where].map(shown));
      expect(
        arrived,
        "these rules set uppercase micro-type themselves instead of carrying " +
          ".t-eyebrow, which is what let five sheets disagree about its size and " +
          "tracking. Add the class to the element and delete the properties",
      ).toEqual([]);
    });

    it("lets no ratified sheet grow another copy", () => {
      const grew = Object.entries(eyebrowBaseline)
        .filter(([where, frozen]) => (bySheet[where]?.length ?? 0) > frozen)
        .map(
          ([where, frozen]) =>
            `${where}: ${bySheet[where].length} now, ${frozen} when frozen — ` +
            bySheet[where].map((f) => f.rule.selector).join(", "),
        );
      expect(grew, "the baseline only shrinks").toEqual([]);
    });

    it("reports a sheet that has been cleaned, so the baseline shrinks", () => {
      const shrunk = Object.entries(eyebrowBaseline)
        .filter(([where, frozen]) => (bySheet[where]?.length ?? 0) < frozen)
        .map(
          ([where, frozen]) =>
            `${where}: ${bySheet[where]?.length ?? 0} now, ${frozen} in the baseline`,
        );
      expect(
        shrunk,
        "these sheets carry fewer copies than the baseline records. Lower the " +
          "number, or delete the line when it reaches zero",
      ).toEqual([]);
    });
  });

  // The detector must be able to SEE each shape, or a clean tree and a broken
  // reader look identical. Planted here rather than trusted.
  describe("the detector", () => {
    const known = utilities(readFileSync(join(frontendRoot, owner), "utf8"));
    const pair = eyebrowPair(known);
    const only = (css: string): Rule => {
      const rules = rulesIn(css);
      expect(rules).toHaveLength(1);
      return rules[0];
    };

    it("reads a rule that says everything .t-caption says as .t-caption", () => {
      const rule = only(
        ".x-note { margin-top: 4px; font-size: var(--fs-meta); color: var(--textMeta); }",
      );
      expect(restates(rule, known)?.name).toBe(".t-caption");
    });

    it("reads the most specific utility a rule spells", () => {
      const rule = only(
        ".x-key { font-size: var(--fs-meta); font-weight: var(--fw-semibold); color: var(--textMeta); }",
      );
      expect(restates(rule, known)?.name).toBe(".t-label");
    });

    it("does not read a size alone, or a size with its own colour, as a utility", () => {
      expect(
        restates(only(".x { font-size: var(--fs-meta); }"), known),
      ).toBeUndefined();
      expect(
        restates(
          only(".x { font-size: var(--fs-meta); color: var(--danger); }"),
          known,
        ),
      ).toBeUndefined();
    });

    it("reads a copy inside a breakpoint, and not a parent for its child", () => {
      const rules = rulesIn(
        "@media (max-width: 600px) { .x-note { font-size: var(--fs-meta); color: var(--textMeta); } }\n" +
          ".parent { display: grid; .child { font-size: var(--fs-sm); color: var(--textMeta); } }",
      );
      expect(rules.map((rule) => rule.selector)).toEqual([
        ".x-note",
        ".child",
        ".parent",
      ]);
      expect(rules.map((rule) => restates(rule, known)?.name)).toEqual([
        ".t-caption",
        ".t-sub",
        undefined,
      ]);
    });

    it("reads uppercase at micro-type size as the eyebrow whatever else it says", () => {
      expect(
        drawsTheEyebrow(
          only(
            ".x-kicker { font-size: var(--fs-eyebrow); text-transform: uppercase; color: var(--textPrimary); }",
          ),
          pair,
        ),
      ).toBe(true);
      expect(
        drawsTheEyebrow(
          only(
            ".x-head { font-size: var(--fs-meta); text-transform: uppercase; }",
          ),
          pair,
        ),
      ).toBe(true);
      expect(
        drawsTheEyebrow(only(".x { font-size: var(--fs-eyebrow); }"), pair),
      ).toBe(false);
      expect(
        drawsTheEyebrow(
          only("th { font-size: var(--fs-sm); text-transform: uppercase; }"),
          pair,
        ),
      ).toBe(false);
    });

    it("reads a waiver on the size, with a reason, and no other", () => {
      expect(
        waiverIn(
          only(
            ".x {\n  /* ds:ignore the host renders this outside the class layer */\n  font-size: var(--fs-meta);\n}",
          ),
        ),
      ).toBe("reasoned");
      expect(
        waiverIn(
          only(
            ".x {\n  font-size: var(--fs-meta); /* ds:ignore a monogram, not a caption */\n}",
          ),
        ),
      ).toBe("reasoned");
      expect(
        waiverIn(
          only(".x {\n  /* ds:ignore */\n  font-size: var(--fs-meta);\n}"),
        ),
      ).toBe("bare");
      // A spacing waiver in the same rule is not this gate's.
      expect(
        waiverIn(
          only(
            ".x {\n  font-size: var(--fs-meta);\n  margin: 4px; /* ds:ignore a hint sits tighter */\n}",
          ),
        ),
      ).toBe("none");
    });
  });
});
