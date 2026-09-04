// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Corner gate: one ladder, and the squircle that rides on it.
//
// The tree ran seven raw radii beside the tokens — 3, 5, 6, 9, 10, 11 and 28px
// — each somebody's judgement of "about right" and together a ladder with no
// rungs. Nothing failed, because no gate read a `border-radius`. This one does,
// and it holds three separate obligations that all decay the same silent way:
//
// 1. **Every corner reads the ladder.** A radius under `frontend/src` (and the
//    extension frontends) is `var(--r-*)`, `0`, `inherit`, `50%`, or a `calc()`
//    over one of those. A raw px value is a rung nobody declared.
// 2. **A round thing opts out of the squircle.** `tokens.css` sets
//    `corner-shape: squircle` on `:where(*)`, so a pill or a circle has to
//    cancel it with `corner-shape: round` beside its own radius. Miss that and
//    an avatar becomes a squircle tile and a status pill a lozenge — a visual
//    defect with no error anywhere.
// 3. **The doubling stays true.** The `@supports (corner-shape: squircle)`
//    block doubles each finite rung, because a superellipse of radius R reads
//    about as round as a circular corner of R/2. A rung added to the ladder and
//    forgotten in that block would tighten only under a browser that has the
//    property — the hardest kind of drift to see, since the two states never
//    appear side by side.
//
// The canon is READ from `tokens.css` rather than restated here: a gate that
// hard-codes part of its subject has become a second copy of it, and this one
// would then be the eighth spelling of the ladder. What is asserted about the
// canon is its SHAPE (a multiple of four, and a doubled twin), which is the
// rule the values obey rather than the values themselves — `tokens.test.ts`
// already pins those against the design canon.
//
// Under-recognition is the failure mode that matters here: a corpus that misses
// files, or a scan that misses declarations inside a media query, reads a
// smaller tree and reports PASS with nothing to notice. So the corpus fails
// closed when it is empty, the scan reads INNERMOST blocks (which is every
// block, at-rule-nested or not), and the detector carries planted cases of each
// shape it must see.

import { existsSync, readdirSync, readFileSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import { describe, expect, it } from "vitest";

const frontendRoot = resolve(__dirname, "..", "..");
const tokensFile = join(frontendRoot, "src", "design-system", "tokens.css");

function cssFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return entry.name === "node_modules" || entry.name === "dist"
        ? []
        : cssFiles(path);
    }
    return entry.name.endsWith(".css") ? [path] : [];
  });
}

// A unit ships CSS into the same page from the same tokens, so it is held to
// the same ladder. Leaving the tier out is the census that fails short.
function extensionStylesheets(): string[] {
  const root = join(frontendRoot, "..", "extensions");
  if (!existsSync(root)) {
    return [];
  }
  return readdirSync(root, { withFileTypes: true }).flatMap((unit) => {
    const frontend = join(root, unit.name, "frontend");
    return unit.isDirectory() && existsSync(frontend) ? cssFiles(frontend) : [];
  });
}

const sheets = cssFiles(join(frontendRoot, "src"))
  .concat(extensionStylesheets())
  .filter((file) => file !== tokensFile);

/** The ladder, as `tokens.css` itself declares it. */
function ladder(tokens: string): Map<string, string> {
  const found = new Map<string, string>();
  const base = tokens.slice(0, tokens.indexOf("@supports"));
  const decl = /(--r-[a-z]+)\s*:\s*([^;]+);/g;
  for (let m = decl.exec(base); m; m = decl.exec(base)) {
    found.set(m[1], m[2].trim());
  }
  return found;
}

/** The same ladder as the squircle block redeclares it. */
function squircleLadder(tokens: string): Map<string, string> {
  const found = new Map<string, string>();
  const at = tokens.indexOf("@supports (corner-shape: squircle)");
  if (at < 0) {
    return found;
  }
  const decl = /(--r-[a-z]+)\s*:\s*([^;]+);/g;
  const block = tokens.slice(at);
  for (let m = decl.exec(block); m; m = decl.exec(block)) {
    found.set(m[1], m[2].trim());
  }
  return found;
}

type Block = { body: string; at: number };

/**
 * Every INNERMOST rule block in a stylesheet. A block nested in `@media` or
 * `@supports` is innermost too, which is the point: a raw radius hides just as
 * well inside a media query, and a scan that skipped at-rules would report PASS
 * over the half of this tree that is responsive.
 */
function blocks(text: string): Block[] {
  const out: Block[] = [];
  const rule = /\{([^{}]*)\}/g;
  for (let m = rule.exec(text); m; m = rule.exec(text)) {
    out.push({ body: m[1], at: m.index });
  }
  return out;
}

type Radius = { property: string; value: string; waived: boolean };

/** The radius declarations in one block, with any in-line waiver noted. */
function radii(body: string): Radius[] {
  const out: Radius[] = [];
  const decl = /(border(?:-[a-z]+)*-radius)\s*:\s*([^;}]+)(;?)([^\n]*)/g;
  for (let m = decl.exec(body); m; m = decl.exec(body)) {
    out.push({
      property: m[1],
      value: m[2].trim(),
      waived: waivedWithReason(m[4] ?? ""),
    });
  }
  return out;
}

/**
 * A waiver is the sentence, not the marker. `/* ds:ignore *\/` alone is itself a
 * finding: it says a rule was skipped and not why, which is the note the next
 * reader cannot act on.
 */
function waivedWithReason(trailing: string): boolean {
  const comment = /\/\*([\s\S]*?)\*\//.exec(trailing);
  if (!comment) {
    return false;
  }
  const reason = comment[1]
    .trim()
    .replace(/^ds:ignore\b/, "")
    .trim();
  return /^ds:ignore\b/.test(comment[1].trim()) && reason.length > 0;
}

/** Stands in for a masked calc() while a shorthand is split on its spaces. */
const CALC = "@calc";

/**
 * True where every corner in a value names the ladder, or is a corner with no
 * radius at all. A shorthand is read corner by corner, because the shorthand
 * with one raw corner among three tokens is exactly how the strays got in.
 */
function readsTheLadder(value: string, rungs: Set<string>): boolean {
  // A calc() is one corner and carries its own spaces, so it is masked before
  // the value is split — and it stands only where it adjusts a rung of the
  // ladder, which is the nested-radius law (inner = outer − padding).
  const calls: string[] = [];
  const masked = value.replace(
    /calc\([^()]*(?:\([^()]*\)[^()]*)*\)/g,
    (whole) => {
      calls.push(whole);
      return `${CALC}${calls.length - 1}`;
    },
  );
  return masked
    .trim()
    .split(/\s+/)
    .filter(Boolean)
    .every((corner) => {
      const call = new RegExp(`^${CALC}(\\d+)$`).exec(corner);
      if (call) {
        return namedRungs(calls[Number(call[1])], rungs).length > 0;
      }
      if (corner === "0" || corner === "inherit" || corner === "50%") {
        return true;
      }
      return (
        namedRungs(corner, rungs).length === 1 &&
        /^var\(--r-[a-z]+\)$/.test(corner)
      );
    });
}

/** The ladder rungs a fragment of a value names. */
function namedRungs(text: string, rungs: Set<string>): string[] {
  const out: string[] = [];
  const use = /var\((--r-[a-z]+)\)/g;
  for (let m = use.exec(text); m; m = use.exec(text)) {
    if (rungs.has(m[1])) {
      out.push(m[1]);
    }
  }
  return out;
}

/** A corner that is genuinely round, and so has to cancel the squircle. */
function isRound(value: string): boolean {
  return /var\(--r-full\)|50%/.test(value);
}

describe("every corner in the tree reads one ladder", () => {
  it("sweeps a corpus that is not empty", () => {
    // A gate pointed at the wrong tree reports PASS forever. This is the only
    // assertion here that fires on a mistake in the gate rather than in the CSS.
    expect(sheets.length).toBeGreaterThan(40);
    expect(existsSync(tokensFile)).toBe(true);
  });

  it("finds no radius outside the ladder", () => {
    const rungs = new Set(ladder(readFileSync(tokensFile, "utf8")).keys());
    const strays: string[] = [];
    for (const file of sheets) {
      for (const block of blocks(readFileSync(file, "utf8"))) {
        for (const radius of radii(block.body)) {
          if (radius.waived || readsTheLadder(radius.value, rungs)) {
            continue;
          }
          strays.push(
            `${relative(frontendRoot, file)}: \`${radius.property}: ${radius.value}\``,
          );
        }
      }
    }
    expect(
      strays,
      `a corner names a length instead of a rung of the ladder. The rungs are ` +
        `${[...rungs].join(", ")}, declared in design-system/tokens.css; pick the ` +
        `nearest one rather than adding an eighth value nobody chose. A genuine ` +
        `one-off is waived in line, with a reason: /* ds:ignore <reason> */\n` +
        strays.join("\n"),
    ).toEqual([]);
  });

  it("cancels the squircle wherever a corner is genuinely round", () => {
    const missing: string[] = [];
    for (const file of sheets) {
      for (const block of blocks(readFileSync(file, "utf8"))) {
        const round = radii(block.body).filter(
          (radius) => isRound(radius.value) && !radius.waived,
        );
        if (round.length === 0 || /corner-shape\s*:\s*round/.test(block.body)) {
          continue;
        }
        missing.push(
          `${relative(frontendRoot, file)}: \`${round[0].property}: ${round[0].value}\``,
        );
      }
    }
    expect(
      missing,
      `a pill or a circle inherits \`corner-shape: squircle\` from tokens.css, ` +
        `which draws it as a lozenge or a squircle tile. Add \`corner-shape: ` +
        `round;\` beside the radius — :where(*) carries no specificity, so that ` +
        `one line always wins.\n` +
        missing.join("\n"),
    ).toEqual([]);
  });

  it("keeps `squircle` itself to the one place that declares it", () => {
    const elsewhere: string[] = [];
    for (const file of sheets) {
      const text = readFileSync(file, "utf8");
      const decl = /corner-shape\s*:\s*([^;}]+)/g;
      for (let m = decl.exec(text); m; m = decl.exec(text)) {
        if (m[1].trim() !== "round") {
          elsewhere.push(`${relative(frontendRoot, file)}: \`${m[0]}\``);
        }
      }
    }
    expect(
      elsewhere,
      `corner-shape is declared once, on :where(*) in design-system/tokens.css. ` +
        `A call site says only \`round\`, to opt a genuinely round thing out — a ` +
        `second squircle declaration is a second answer to what the house corner ` +
        `is.\n` +
        elsewhere.join("\n"),
    ).toEqual([]);
  });
});

describe("the ladder itself", () => {
  const tokens = readFileSync(tokensFile, "utf8");

  it("runs in fours", () => {
    for (const [name, value] of ladder(tokens)) {
      if (name === "--r-full") {
        continue;
      }
      const px = Number(value.replace("px", ""));
      expect(
        Number.isInteger(px) && px % 4 === 0,
        `${name} is ${value}. The ladder runs in fours — 4 / 8 / 12 / 16 / 20 — ` +
          `so that a corner is a rung a reader can name rather than a number ` +
          `somebody liked.`,
      ).toBe(true);
    }
  });

  it("doubles every finite rung under corner-shape, and the pill not at all", () => {
    const base = ladder(tokens);
    const smooth = squircleLadder(tokens);
    expect(smooth.size).toBeGreaterThan(0);
    for (const [name, value] of base) {
      if (name === "--r-full") {
        expect(
          smooth.has(name),
          `--r-full is redeclared in the squircle block. It is already larger ` +
            `than any box it lands on, and doubling it says nothing; the pill ` +
            `stays round by cancelling the shape, not by growing.`,
        ).toBe(false);
        continue;
      }
      const doubled = smooth.get(name);
      expect(
        doubled,
        `${name} has no twin in the @supports (corner-shape: squircle) block. ` +
          `A rung that is not doubled there tightens the day a browser ships the ` +
          `property, and only there — the two states never appear side by side, ` +
          `so nothing would look wrong here.`,
      ).toBeDefined();
      expect(doubled).toBe(`${Number(value.replace("px", "")) * 2}px`);
    }
  });
});

describe("the corner detector sees what it claims to", () => {
  const rungs = new Set([
    "--r-xs",
    "--r-sm",
    "--r-control",
    "--r-md",
    "--r-lg",
    "--r-full",
  ]);

  const cases: { name: string; css: string; strays: number }[] = [
    {
      name: "a plain token",
      css: ".a { border-radius: var(--r-md); }",
      strays: 0,
    },
    { name: "a raw px", css: ".a { border-radius: 11px; }", strays: 1 },
    {
      name: "a raw px inside a media query",
      css: "@media (max-width: 40rem) { .a { border-radius: 11px; } }",
      strays: 1,
    },
    {
      name: "a raw px on one corner only",
      css: ".a { border-bottom-right-radius: 3px; }",
      strays: 1,
    },
    {
      name: "a token nested by a calc",
      css: ".a { border-radius: calc(var(--r-sm) - 2px); }",
      strays: 0,
    },
    {
      name: "a mixed shorthand with one raw corner",
      css: ".a { border-radius: 4px var(--r-md) var(--r-md) var(--r-md); }",
      strays: 1,
    },
    {
      name: "a zero and a keyword",
      css: ".a { border-radius: 0; } .b { border-radius: inherit; }",
      strays: 0,
    },
    { name: "a circle", css: ".a { border-radius: 50%; }", strays: 0 },
    {
      name: "a waived one-off",
      css: ".a { border-radius: 3px; /* ds:ignore the notch's own tip */ }",
      strays: 0,
    },
    {
      name: "a waiver with no reason",
      css: ".a { border-radius: 3px; /* ds:ignore */ }",
      strays: 1,
    },
    {
      // The name is assembled rather than written out. Spelled in full it would
      // read as a USE of a custom property nothing declares, and
      // check-space-tokens.sh sweeps the test tree too — the probe is about the
      // detector, not a property this product has.
      name: "a token that is not on the ladder",
      css: `.a { border-radius: var(${"--radius-lg"}); }`,
      strays: 1,
    },
  ];

  for (const probe of cases) {
    it(`${probe.strays ? "reports" : "ignores"} ${probe.name}`, () => {
      const strays = blocks(probe.css)
        .flatMap((block) => radii(block.body))
        .filter(
          (radius) => !radius.waived && !readsTheLadder(radius.value, rungs),
        );
      expect(strays).toHaveLength(probe.strays);
    });
  }

  it("reads a round corner in every spelling it is written in", () => {
    expect(isRound("var(--r-full)")).toBe(true);
    expect(isRound("50%")).toBe(true);
    expect(isRound("var(--r-md)")).toBe(false);
  });

  it("sees a block that opts out, and one that forgot to", () => {
    const kept = blocks(".a { border-radius: 50%; corner-shape: round; }")[0];
    const missed = blocks(".a { border-radius: 50%; }")[0];
    expect(/corner-shape\s*:\s*round/.test(kept.body)).toBe(true);
    expect(/corner-shape\s*:\s*round/.test(missed.body)).toBe(false);
  });
});
