import { readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { extensionLayers, filesMatching } from "../../scripts/lib/source-tree";

// Depth, source-wide. This product has exactly three of it, and they answer
// three different questions about one light source above:
//
//   --shadow-rest  a surface or a filled verb has a TOP SIDE
//   --shadow-well  a field is a place to put something, so it has a FLOOR —
//                  the same geometry as --shadow-rest, turned inside
//   --shadow-pop   a popover, menu or drawer is genuinely above the plane
//
// A control closes the gap on hover, active and focus: a verb presses into the
// page, a field's floor comes up to meet the pointer, and either way the layer
// goes to `none`. That is the whole of the interaction.
//
// The token is what makes that changeable: change --shadow-rest and every
// resting surface follows, in both themes, from one line. A rule that spells
// its own shadow does not follow, and nothing reports it — a shadow is 5% of
// one colour over two pixels, so a surface left at the old depth looks like
// every other surface to anyone reading a diff or a screenshot. That is the
// class of defect a gate exists for and an eye does not catch.
//
// The allowlist below is what a box-shadow is used for here OTHER than depth:
// rings, halos, and one full-bleed fill trick. Each entry says why, and a new
// one has to as well — the question a reason answers is "why is this not the
// token", and "it looked right" is not an answer to it.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

// The corpus is the shared walk, not a private one: a second walker is a second
// answer to node_modules, to symlinks and to a nested extension layer, and the
// way that shows up is a gate reading a SMALLER tree and reporting PASS.
//
// A unit's screen is shipped UI in the same bundle on the same page, so the
// extension tier is swept with the core. Leaving it out would hold the core to
// a depth rule the tier escapes.
const CSS = /\.css$/;
const cssFiles = [
  ...filesMatching(join(frontendRoot, "src"), CSS),
  ...extensionLayers(join(frontendRoot, "..", "extensions")).flatMap((layer) =>
    filesMatching(layer, CSS),
  ),
];

// `none` and `0` are the suppression half of the pattern — a control with no
// fill taking the resting layer back off, which is a decision and not a drift.
const HOUSE_SHADOW =
  /^(none|0|var\(--shadow-(rest|well|pop)\)|var\(--focus-glow(-danger|-ai)?\))$/;

/**
 * Whether every layer of a shadow is a RULE rather than depth.
 *
 * Every inset in this tree is a rail or a ring — a 3px bar down one edge, a 1px
 * ring inside the box — drawn inside rather than as a border so it costs the
 * element no size. None of them says how high anything sits, so none of them is
 * this gate's business.
 *
 * Depth is what has BLUR, and now that --shadow-well exists a blurred inset is
 * an inset well somebody spelled by hand. So the blur — the third length — must
 * be zero or absent, and it is asked of EVERY comma-separated layer rather than
 * of the value's first word: the orb's chrome begins with a flat inset ring and
 * then adds an outer bloom, so a check that read only the opening would wave a
 * 44px glow through on the strength of the ring in front of it.
 */
function drawsOnlyFlatInsets(value: string): boolean {
  return topLevelLayers(value).every((layer) => {
    if (!/^inset\b/.test(layer)) {
      return false;
    }
    const lengths = layer
      .slice("inset".length)
      .trim()
      .split(/\s+/)
      .filter((token) => /^-?(\d*\.)?\d+(px|rem|em)?$/.test(token));
    // Fewer than three lengths is offset-only: no blur to be depth with.
    return lengths.length < 3 || /^0(px|rem|em)?$/.test(lengths[2]);
  });
}

/**
 * One shadow's comma-separated layers, splitting only at paren depth zero —
 * `color-mix(in srgb, …)` carries commas of its own, and a naive split would
 * tear a colour in half and judge the pieces.
 */
function topLevelLayers(value: string): string[] {
  const layers: string[] = [];
  let depth = 0;
  let current = "";
  for (const character of value) {
    if (character === "(") depth += 1;
    if (character === ")") depth -= 1;
    if (character === "," && depth === 0) {
      layers.push(current.trim());
      current = "";
      continue;
    }
    current += character;
  }
  layers.push(current.trim());
  return layers.filter((layer) => layer !== "");
}

/**
 * Every box-shadow VALUE in one stylesheet that is not one of the house
 * spellings, as `path: box-shadow: <value>`.
 *
 * Statements, not lines: a multi-layer shadow is wrapped by the formatter, so a
 * gate reading lines would see half of each one and judge neither. Comments go
 * first, or a sentence about a shadow is read as one — this tree explains its
 * depth decisions at length, and those paragraphs are not declarations.
 */
function spelledShadows(file: string): string[] {
  const text = readFileSync(file, "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
  return [...text.matchAll(/box-shadow:\s*([^;}]+)[;}]/g)]
    .map((match) => match[1].trim().replace(/\s+/g, " "))
    .filter((value) => !HOUSE_SHADOW.test(value) && !drawsOnlyFlatInsets(value))
    .map((value) => `${relative(frontendRoot, file)}: box-shadow: ${value}`);
}

describe("depth", () => {
  it("draws every shadow from the three depth tokens", () => {
    const spelled = cssFiles
      // tokens.css is where the two values LIVE; a literal there is its job.
      .filter((file) => !file.endsWith("tokens.css"))
      .flatMap(spelledShadows)
      // Sorted, because the walk's order is the filesystem's and this list has
      // to mean the same thing on a developer's machine and in CI.
      .sort();
    expect(
      spelled,
      "a shadow reads `var(--shadow-rest)` for a top side, " +
        "`var(--shadow-well)` for a field's floor, or `var(--shadow-pop)` " +
        "above the plane; these spell their own geometry and colour, which " +
        "is how one depth becomes twenty that no longer agree",
    ).toEqual([
      // The agent's state mark and its recap marks are LIGHTS, not boxes: a 5px
      // dot with its own halo in the tone the orb is running, so the panel and
      // the ball read as one object. A halo is the mark's own colour spreading.
      "src/app/agentrail.css: box-shadow: 0 0 6px -1px var(--arTone)",
      "src/app/agentrail.css: box-shadow: 0 0 6px -1px var(--arTone)",
      // The ring separating two overlapping faces, in the colour of the surface
      // behind them. A shadow rather than a border so a stacked face stays the
      // size of the lone chip beside it (avatarstack.css says why).
      "src/design-system/avatarstack.css: box-shadow: 0 0 0 2px var(--bgElevated)",
      // The verdict a reader just gave a drafted card, held as a coloured ring
      // until the card springs back. A ring rather than a tint because the card
      // underneath carries prose somebody is still reading.
      "src/design-system/decisiondeck.css: box-shadow: 0 0 0 2px var(--accent)",
      "src/design-system/decisiondeck.css: box-shadow: 0 0 0 2px var(--ai)",
      "src/design-system/decisiondeck.css: box-shadow: 0 0 0 2px var(--borderStrong)",
      "src/design-system/decisiondeck.css: box-shadow: 0 0 0 2px var(--danger)",
      // The agent orb is a lit SPHERE, not a surface at a depth: an inner
      // shading gradient, a white hairline just inside the edge, and the ball's
      // own bloom, modelling one object under one light. The second value is
      // the same chrome at the small container rung. Neither is an elevation
      // and no token could carry either.
      "src/design-system/margince-core.css: box-shadow: inset 0 -40px 60px -46px var(--coreC1), inset 0 0 0 1px color-mix(in srgb, var(--overlayLight) 8%, transparent), 0 0 44px -22px var(--coreC2)",
      "src/design-system/margince-core.css: box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--overlayLight) 6%, transparent), 0 0 12px -7px var(--coreC2)",
      // The capture mark while the agent is reading: a halo in the same tint as
      // the mark's fill. That is the state, not an elevation.
      "src/screens/backfill.css: box-shadow: 0 0 0 4px var(--aiLight)",
      // The relay step the reader is on, ringed the same way — a position in a
      // sequence rather than a lift off the page.
      "src/screens/contactnetwork.css: box-shadow: 0 0 0 4px var(--accentLight)",
      // The pulse marking the finding a jump just landed on: a ring that holds
      // for a beat and fades, spelled as a box-shadow specifically so it layers
      // BESIDE the browser's own focus ring instead of fighting it for the same
      // property. Four values: the keyframe set, plus the static ring that
      // replaces it under reduced motion.
      "src/screens/onboarding-conversation/conversation.css: box-shadow: 0 0 0 2px var(--aiText)",
      "src/screens/onboarding-conversation/conversation.css: box-shadow: 0 0 0 3px transparent",
      "src/screens/onboarding-conversation/conversation.css: box-shadow: 0 0 0 3px var(--aiText)",
      "src/screens/onboarding-conversation/conversation.css: box-shadow: 0 0 0 3px var(--aiText)",
      // Not a shadow at all: a sticky bar's full-bleed fill and the hairline
      // behind it, painted as two spreads because a spread is the only way to
      // reach past the bar's own box. conversation.css spells out the ordering
      // that keeps the hairline alive in exactly one pixel.
      "src/screens/onboarding-conversation/conversation.css: box-shadow: 0 calc(100vw + 1px) 0 100vw var(--bgElevated), 0 0 0 100vw var(--borderSubtle)",
      // The live dot and the active journey step on the onboarding rail: lights
      // again, glowing in their own ink on that surface's dark brand ground.
      "src/screens/onboarding.css: box-shadow: 0 0 12px var(--aiText)",
      "src/screens/onboarding.css: box-shadow: 0 0 16px var(--aiMed)",
      // The one genuine drop shadow outside --shadow-pop, and the reason is the
      // ground: this dialog floats on the rail's own dark brand surface, where
      // --shadow-pop is tinted for the page and reads as a grey smear. Tinted
      // to --railBottom so it is the shadow that ground would actually cast.
      "src/screens/onboarding.css: box-shadow: 0 18px 46px color-mix(in srgb, var(--railBottom) 44%, transparent)",
    ]);
  });

  // The census that must not fail short. An empty corpus reports PASS with
  // nothing asserted, so a walk that lost the tree — a moved directory, a
  // pattern that stopped matching — would read exactly like a clean one.
  it("scans the stylesheets it claims to", () => {
    expect(
      cssFiles.length,
      "the depth sweep found almost no stylesheets, which is a broken walk " +
        "rather than a tree without shadows",
    ).toBeGreaterThan(50);
  });

  // The other way this gate fails short. Reading a custom property nothing
  // declares is not a CSS error: the declaration is dropped and the shadow
  // simply stops being drawn, in silence, on every surface at once. Every rule
  // above reads the name, so the name has to exist in every theme arm — not
  // only in the one whose screenshot somebody happened to look at.
  it.each(["--shadow-rest", "--shadow-well", "--shadow-pop"])(
    "declares %s in every theme arm",
    (token) => {
      const tokens = readFileSync(
        join(frontendRoot, "src", "design-system", "tokens.css"),
        "utf8",
      );
      expect(
        [...tokens.matchAll(new RegExp(`${token}:\\s*[^;]+;`, "g"))].length,
        `${token} belongs in the light block, the dark toggle and the ` +
          "platform-preference arm; a theme missing it loses that depth " +
          "everywhere at once, and says nothing about it",
      ).toBe(3);
    },
  );
});
