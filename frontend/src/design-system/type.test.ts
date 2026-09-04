// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Type gate: one ramp, and one tracking family.
//
// `tokens.css` declared a scale and the tree wrote 25 other sizes beside it —
// 12.5px in 25 places, 11px in 22, 11.5px in 10, none of them a rung anybody
// had chosen. Nothing failed, because no gate read a `font-size`. The three
// gates in this neighbourhood each hold a different property: check-font-lock
// reads `font-family`, check-ds-purity reads colour, check-space-tokens reads
// custom properties. A rule that typed `11.5px` passed all three.
//
// Two obligations, and both decay silently:
//
// 1. **Every size names a rung.** A `font-size` in a stylesheet, or a
//    `fontSize` in an inline style, is `var(--fs-*)`, `inherit`, or a `clamp()`
//    whose ends are rungs. A raw length is a size nobody declared, and it is
//    invisible in review — 12.5px next to 13px looks like agreement.
// 2. **Every tracking names the family.** `--tracking-eyebrow`,
//    `--tracking-display`, `--tracking-normal`. The eyebrow token said 0.08em
//    while eighteen hand-written copies ran 0.02em to 0.14em, so the label a
//    reader saw depended on which sheet drew it.
//
// The canon is READ from `tokens.css`. A gate that restated the ramp would
// become the fifth copy of it, which is the defect this file exists to stop.
// What it asserts about the canon itself is its SHAPE — the rungs ascend, and
// no two of them are the same size — because two rungs at one value is how a
// scale stops being a scale. `tokens.test.ts` pins the values.
//
// The waiver is `ds:ignore <reason>` on the declaration, the spelling
// check-ds-spacing already taught the tree. Two live today and both are a
// PLATFORM fact rather than a design choice: iOS zooms the viewport on any
// field under 16px, and the mask glyph's dot gap is a picture rather than a
// label. A waiver with no reason is itself a finding.
//
// Under-recognition is the way this gate must not break. A corpus that misses
// files, or a scan that skips a media query, reads a smaller tree and reports
// PASS with nothing to assert. So the corpus fails closed when it is empty, the
// scan reads the raw text rather than top-level rules, and the detector carries
// planted cases of every shape it has to see.

import { existsSync, readdirSync, readFileSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import { describe, expect, it } from "vitest";

const frontendRoot = resolve(__dirname, "..", "..");
const tokensFile = join(frontendRoot, "src", "design-system", "tokens.css");

function sourceFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return entry.name === "node_modules" || entry.name === "dist"
        ? []
        : sourceFiles(path);
    }
    return /\.(css|tsx)$/.test(entry.name) ? [path] : [];
  });
}

// A unit's screen ships into the same page and is held to the same ramp.
function extensionFrontends(): string[] {
  const root = join(frontendRoot, "..", "extensions");
  if (!existsSync(root)) {
    return [];
  }
  return readdirSync(root, { withFileTypes: true }).flatMap((unit) => {
    const frontend = join(root, unit.name, "frontend");
    return unit.isDirectory() && existsSync(frontend)
      ? sourceFiles(frontend)
      : [];
  });
}

const files = sourceFiles(join(frontendRoot, "src"))
  .concat(extensionFrontends())
  .filter((file) => file !== tokensFile);

/** The scale, as `tokens.css` itself declares it. */
function scale(tokens: string, prefix: string): Map<string, string> {
  const found = new Map<string, string>();
  const decl = new RegExp(`(--${prefix}-[a-z0-9-]+)\\s*:\\s*([^;]+);`, "g");
  for (let m = decl.exec(tokens); m; m = decl.exec(tokens)) {
    found.set(m[1], m[2].split("/*")[0].trim());
  }
  return found;
}

type Use = { property: string; value: string; waived: boolean };

/**
 * Every size or tracking declaration in a file, CSS and inline style alike,
 * with any waiver on the line noted. The two spellings are read together
 * because they are one obligation — an inline `fontSize` is as visible to a
 * reader as a stylesheet's, and only invisible to a gate that forgot it.
 */
function uses(text: string, css: string, jsx: string): Use[] {
  const out: Use[] = [];
  const patterns = [
    new RegExp(`(${css})\\s*:\\s*([^;}\\n]+)([^\\n]*)`, "g"),
    new RegExp(`(${jsx})\\s*:\\s*("[^"]*"|[0-9.]+)\\s*,?([^\\n]*)`, "g"),
  ];
  for (const pattern of patterns) {
    for (let m = pattern.exec(text); m; m = pattern.exec(text)) {
      out.push({
        property: m[1],
        value: m[2].trim().replace(/^"|"$/g, ""),
        waived: waivedWithReason(m[3] ?? ""),
      });
    }
  }
  return out;
}

/**
 * A waiver is the sentence, not the marker: `ds:ignore` alone says a rule was
 * skipped and not why, which is the note the next reader cannot act on. Both
 * comment spellings count, because both trees are swept.
 */
function waivedWithReason(trailing: string): boolean {
  const comment = /(?:\/\*|\/\/)\s*ds:ignore\b([^*\n]*)/.exec(trailing);
  return comment ? comment[1].replace(/\*\//, "").trim().length > 0 : false;
}

/** True where a value names only the scale, or inherits one. */
function readsTheScale(value: string, rungs: Set<string>): boolean {
  if (value === "inherit" || value === "unset") {
    return true;
  }
  const names = [...value.matchAll(/var\((--[a-z0-9-]+)\)/g)].map((m) => m[1]);
  if (names.length === 0) {
    return false;
  }
  if (!names.every((name) => rungs.has(name))) {
    return false;
  }
  // A clamp() is the scale made fluid: its ends are rungs and the middle is a
  // viewport rule. What is refused is a clamp with a raw length at an end,
  // which is a rung nobody declared wearing a responsive coat.
  return !/(^|[\s(,])[0-9.]+(px|rem|em)/.test(value);
}

describe("every size in the tree names a rung", () => {
  const rungs = new Set(scale(readFileSync(tokensFile, "utf8"), "fs").keys());

  it("sweeps a corpus that is not empty", () => {
    expect(files.length).toBeGreaterThan(200);
    expect(rungs.size).toBeGreaterThan(5);
  });

  it("finds no font-size outside the ramp", () => {
    const strays: string[] = [];
    for (const file of files) {
      const text = readFileSync(file, "utf8");
      for (const use of uses(text, "font-size", "fontSize")) {
        if (use.waived || readsTheScale(use.value, rungs)) {
          continue;
        }
        strays.push(
          `${relative(frontendRoot, file)}: \`${use.property}: ${use.value}\``,
        );
      }
    }
    expect(
      strays,
      `a size names a length instead of a rung of the ramp. The rungs are ` +
        `${[...rungs].join(", ")}, declared in design-system/tokens.css; pick the ` +
        `nearest one. 12.5px beside 13px is not a decision a reader can see, and ` +
        `the tree carried 25 such values before this gate. A genuine platform ` +
        `constraint is waived in line, with a reason: /* ds:ignore <reason> */\n` +
        strays.join("\n"),
    ).toEqual([]);
  });

  it("finds no tracking outside the family", () => {
    const family = new Set(
      scale(readFileSync(tokensFile, "utf8"), "tracking").keys(),
    );
    const strays: string[] = [];
    for (const file of files) {
      const text = readFileSync(file, "utf8");
      for (const use of uses(text, "letter-spacing", "letterSpacing")) {
        if (use.waived || readsTheScale(use.value, family)) {
          continue;
        }
        strays.push(
          `${relative(frontendRoot, file)}: \`${use.property}: ${use.value}\``,
        );
      }
    }
    expect(
      strays,
      `tracking is a family of three — ${[...family].join(", ")} — and a ` +
        `hand-typed em is a fourth value on a label that already has a name. ` +
        `The eyebrow ran 0.02em to 0.14em across eighteen sheets while the ` +
        `token said 0.08em.\n` +
        strays.join("\n"),
    ).toEqual([]);
  });
});

describe("the ramp itself", () => {
  const rungs = scale(readFileSync(tokensFile, "utf8"), "fs");

  it("ascends, with no two rungs at one size", () => {
    // The two fluid rungs are a clamp of the fixed ones and have no single
    // size to sort by; what holds them is the size arm above, which reads the
    // ends of a clamp exactly as it reads a plain length.
    const px = [...rungs]
      .filter(([, value]) => /^[0-9.]+px$/.test(value))
      .map(([name, value]) => ({ name, px: Number(value.replace("px", "")) }));
    const sorted = [...px].sort((a, b) => a.px - b.px);
    expect(
      px.map((rung) => rung.name),
      "the rungs are declared out of order, which reads as a scale that has been " +
        "edited rather than designed",
    ).toEqual(sorted.map((rung) => rung.name));
    expect(
      new Set(px.map((rung) => rung.px)).size,
      "two rungs share a size. A scale with a repeated value has a rung nobody " +
        "can choose between, and the next author picks by name at random",
    ).toBe(px.length);
  });
});

describe("the size detector sees what it claims to", () => {
  const rungs = new Set([
    "--fs-eyebrow",
    "--fs-meta",
    "--fs-sm",
    "--fs-body",
    "--fs-h1",
    "--fs-display",
  ]);

  const cases: { name: string; text: string; strays: number }[] = [
    {
      name: "a plain token",
      text: ".a { font-size: var(--fs-sm); }",
      strays: 0,
    },
    { name: "a raw px", text: ".a { font-size: 12.5px; }", strays: 1 },
    {
      name: "a raw px inside a media query",
      text: "@media (max-width: 40rem) { .a { font-size: 11px; } }",
      strays: 1,
    },
    { name: "a rem", text: ".a { font-size: 0.85rem; }", strays: 1 },
    { name: "an em", text: ".a { font-size: 1.05em; }", strays: 1 },
    {
      name: "a clamp built from rungs",
      text: ".a { font-size: clamp(var(--fs-h1), 2.2vw, var(--fs-display)); }",
      strays: 0,
    },
    {
      name: "a clamp with a raw end",
      text: ".a { font-size: clamp(27px, 2.2vw, var(--fs-display)); }",
      strays: 1,
    },
    {
      name: "an inline style in a component",
      text: 'const s = { fontSize: "0.9rem", opacity: 0.85 };',
      strays: 1,
    },
    {
      name: "an inline style that names a rung",
      text: 'const s = { fontSize: "var(--fs-body)" };',
      strays: 0,
    },
    {
      name: "a bare number in an inline style",
      text: "const s = { fontSize: 13, opacity: 0.75 };",
      strays: 1,
    },
    {
      name: "an inherited size",
      text: ".a { font-size: inherit; }",
      strays: 0,
    },
    {
      name: "a waived platform floor",
      text: ".a { font-size: 16px; /* ds:ignore iOS zooms under 16px */ }",
      strays: 0,
    },
    {
      name: "a waiver with no reason",
      text: ".a { font-size: 16px; /* ds:ignore */ }",
      strays: 1,
    },
    {
      name: "a token that is not on the ramp",
      text: `.a { font-size: var(${"--text-lg"}); }`,
      strays: 1,
    },
  ];

  for (const probe of cases) {
    it(`${probe.strays ? "reports" : "ignores"} ${probe.name}`, () => {
      const strays = uses(probe.text, "font-size", "fontSize").filter(
        (use) => !use.waived && !readsTheScale(use.value, rungs),
      );
      expect(strays).toHaveLength(probe.strays);
    });
  }

  it("reads the tracking family with the same eyes", () => {
    const family = new Set(["--tracking-eyebrow", "--tracking-normal"]);
    const seen = uses(
      '.a { letter-spacing: 0.04em; }\nconst s = { letterSpacing: "var(--tracking-eyebrow)" };',
      "letter-spacing",
      "letterSpacing",
    );
    expect(seen).toHaveLength(2);
    expect(
      seen.filter((use) => !readsTheScale(use.value, family)),
    ).toHaveLength(1);
  });

  it("admits a waiver written as a line comment beside an inline style", () => {
    const seen = uses(
      "const s = { fontSize: 13 }; // ds:ignore the mask glyph is a picture",
      "font-size",
      "fontSize",
    );
    expect(seen[0].waived).toBe(true);
  });
});
