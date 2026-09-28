import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import {
  type AvatarMesh,
  allowedArcs,
  FORBIDDEN_HUES,
  HUE_SPREAD,
  hashOf,
  huesAt,
  meshOf,
  meshStyle,
} from "./avatarmesh";
import {
  blocks,
  composite,
  contrastOf,
  hexOf,
  linearOf,
  linearToOklab,
  oklabToLinear,
  parseBlock,
  themes,
  tokenDecls,
} from "./tokens-testing";

const here = dirname(fileURLToPath(import.meta.url));

// The painter as atoms.css declares it: the background's colour layers top to
// bottom (every one but the last fades to nothing) and the ink. Read, not
// restated, so a layer added to the sheet is a layer this suite measures.
const painter = (() => {
  const sheet = readFileSync(join(here, "atoms.css"), "utf8").replace(
    /\/\*[\s\S]*?\*\//g,
    "",
  );
  const rule = /\.avatar-mesh\s*\{([^}]*)\}/.exec(sheet)?.[1] ?? "";
  const background = /background:([^;]*);/.exec(rule)?.[1] ?? "";
  const layers = [...background.matchAll(/var\((--avatar[A-Z]\w*)\)/g)].map(
    (m) => m[1],
  );
  const ink = /(?:^|;)\s*color:\s*var\((--[\w-]+)\)/.exec(rule)?.[1];
  if (layers.length < 2 || !ink) {
    throw new Error("atoms.css: .avatar-mesh no longer reads as layers + ink");
  }
  return { layers, ink };
})();

// Each colour the chip assembles, as the three custom properties it reads.
const assembled = parseBlock(tokenDecls, ":where(.avatar-mesh)");

function colourOf(
  name: string,
  theme: keyof typeof blocks,
  mesh: AvatarMesh,
): number[] {
  const parts =
    /^oklch\(\s*var\((--\w+)\)\s+var\((--\w+)\)\s+var\((--[\w-]+)\)\s*\)$/.exec(
      assembled[name] ?? "",
    );
  if (!parts) {
    throw new Error(`${name} is not oklch(var(L) var(C) var(hue))`);
  }
  const lightness = Number(blocks[theme][parts[1]]);
  const chroma = Number(blocks[theme][parts[2]]);
  const hue = Number(
    Object.entries(meshStyle(mesh)).find(([prop]) => prop === parts[3])?.[1],
  );
  if ([lightness, chroma, hue].some(Number.isNaN)) {
    throw new Error(`${name} does not resolve to numbers in ${theme}`);
  }
  const radians = (hue * Math.PI) / 180;
  return oklabToLinear([
    lightness,
    chroma * Math.cos(radians),
    chroma * Math.sin(radians),
  ]);
}

const rgbaOf = (linear: number[], alpha: number) => {
  const hex = hexOf(linear).slice(1);
  const [r, g, b] = [0, 2, 4].map((i) =>
    Number.parseInt(hex.slice(i, i + 2), 16),
  );
  return `rgba(${r}, ${g}, ${b}, ${alpha})`;
};

// Every point the chip can paint: each fading layer at every share of its
// fade, stacked over the opaque base in the order the sheet stacks them.
function everyPoint(grounds: number[][]): string[] {
  const steps = Array.from({ length: 11 }, (_, i) => i / 10);
  let points = [rgbaOf(grounds[grounds.length - 1], 1)];
  for (const layer of grounds.slice(0, -1).reverse()) {
    points = points.flatMap((under) =>
      steps.map((alpha) => composite(rgbaOf(layer, alpha), under)),
    );
  }
  return points;
}

// The hue range, sampled: every stretch of the allowed wheel, at both extreme
// spreads, turning both ways.
const sampled: AvatarMesh[] = Array.from({ length: 200 }, (_, i) => i / 200)
  .flatMap((position) =>
    [HUE_SPREAD.min, HUE_SPREAD.max].flatMap((spread) =>
      [true, false].map((clockwise) => huesAt(position, spread, clockwise)),
    ),
  )
  .map((hues) => ({ ...meshOf("placement"), ...hues }));

const forbidden = (hue: number) =>
  FORBIDDEN_HUES.some(({ from, to }) => hue > from && hue < to);

function hueOf(value: string): { hue: number; chroma: number } {
  const [, a, b] = linearToOklab(linearOf(value));
  const hue = (Math.atan2(b, a) * 180) / Math.PI;
  return { hue: (hue + 360) % 360, chroma: Math.hypot(a, b) };
}

describe("the key", () => {
  it("is FNV-1a over the code points", () => {
    expect(hashOf("")).toBe(0x811c9dc5);
    expect(hashOf("a")).toBe(0xe40c292c);
    expect(hashOf("foobar")).toBe(0xbf9cf968);
  });

  it("reads a composed and a decomposed letter as one key", () => {
    expect(hashOf("M\u00fcller")).toBe(hashOf("Mu\u0308ller"));
  });

  // Six tones summed over code points put a third of a list on one colour;
  // keys that differ by one character have to land all over the wheel.
  it("spreads sequential ids evenly over the allowed wheel", () => {
    const whole = (bucket: number) =>
      allowedArcs().some(
        ([from, to]) =>
          (bucket * 30 >= from && bucket * 30 + 30 <= to) ||
          (bucket * 30 + 360 >= from && bucket * 30 + 390 <= to),
      );
    const buckets = new Array(12).fill(0);
    for (let n = 0; n < 6000; n += 1) {
      buckets[Math.floor(meshOf(`contact_${n}`).hueA / 30)] += 1;
    }
    const counted = buckets.filter((_, bucket) => whole(bucket));
    expect(counted.length).toBeGreaterThanOrEqual(6);
    expect(Math.max(...counted) / Math.min(...counted)).toBeLessThan(1.3);
  });
});

describe("the hues", () => {
  // The band edges are a mirror of the sheet: each is its family's hue span
  // in tokens.css, widened by the band's margin and rounded outward.
  it.each(FORBIDDEN_HUES)(
    "keeps the $family band where tokens.css puts it",
    (band) => {
      const hues = (["light", "dark"] as const).flatMap((theme) =>
        Object.entries(themes[theme])
          .filter(([name]) => name.startsWith(`--${band.family}`))
          .map(([, value]) => hueOf(value))
          .filter(({ chroma }) => chroma >= 0.05)
          .map(({ hue }) => hue),
      );
      expect(hues.length).toBeGreaterThan(1);
      expect(band.from).toBe(Math.floor(Math.min(...hues) - band.margin));
      expect(band.to).toBe(Math.ceil(Math.max(...hues) + band.margin));
    },
  );

  it("never lands either blob or the base in a forbidden band", () => {
    const keyed = Array.from({ length: 2000 }, (_, n) => meshOf(`record_${n}`));
    for (const mesh of [...sampled, ...keyed]) {
      for (const hue of [mesh.hueA, mesh.hueB, mesh.hueBase]) {
        expect(forbidden(hue), `hue ${hue} of ${JSON.stringify(mesh)}`).toBe(
          false,
        );
      }
    }
  });

  // The narrowest arc cannot hold the full spread from its middle, so there
  // blob B takes the room it has — never less than half that arc.
  it("sets blob B its spread away from blob A, on either side", () => {
    const narrowest = Math.min(...allowedArcs().map(([from, to]) => to - from));
    const floor = Math.min(HUE_SPREAD.min, narrowest / 2);
    const turns = sampled.map(
      ({ hueA, hueB }) => ((hueB - hueA + 540) % 360) - 180,
    );
    for (const turn of turns) {
      expect(Math.abs(turn)).toBeGreaterThanOrEqual(floor - 0.2);
      expect(Math.abs(turn)).toBeLessThanOrEqual(HUE_SPREAD.max + 0.2);
    }
    expect(turns.some((turn) => turn > 0)).toBe(true);
    expect(turns.some((turn) => turn < 0)).toBe(true);
  });
});

describe.each(["light", "dark"] as const)("the %s mesh", (theme) => {
  it("stays inside sRGB at every allowed hue", () => {
    for (const mesh of sampled) {
      for (const name of [...painter.layers, painter.ink]) {
        const linear = colourOf(name, theme, mesh);
        expect(
          linear.every((c) => c >= -0.001 && c <= 1.001),
          `${name} at ${JSON.stringify(mesh)} leaves sRGB`,
        ).toBe(true);
      }
    }
  });

  it("carries its initials at 4.5:1 on every point of the ground", () => {
    let worst = Number.POSITIVE_INFINITY;
    for (const mesh of sampled) {
      const ink = hexOf(colourOf(painter.ink, theme, mesh));
      const grounds = painter.layers.map((name) => colourOf(name, theme, mesh));
      for (const point of everyPoint(grounds)) {
        worst = Math.min(worst, contrastOf(ink, point));
      }
    }
    expect(worst).toBeGreaterThanOrEqual(4.5);
  });
});
