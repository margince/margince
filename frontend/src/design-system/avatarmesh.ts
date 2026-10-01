// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { CSSProperties } from "react";

// The monogram's ground: two soft blobs in neighbouring hues over a base tint,
// every number drawn from the record's key so a record looks the same on every
// screen and in every session. Only NUMBERS leave this file; the lightness and
// chroma that turn them into colour are per-theme tokens in tokens.css.

/**
 * Hues a monogram may never land on, in OKLCh degrees. Each is its family's
 * hue span in tokens.css widened by its margin; avatarmesh.test.ts derives the
 * span from the sheet and fails when an edge here stops matching it.
 */
export const FORBIDDEN_HUES = [
  // `--ai*` spans 275–280.4°. Indigo is a claim that an agent wrote something,
  // and a pale indigo wash is exactly what `--aiLight` paints, so the margin is
  // wide enough to keep blue-violet out as well.
  { family: "ai", margin: 30, from: 245, to: 311 },
  // `--danger*` spans 24.6–25.8°. Red's claim is carried by saturation, so the
  // margin is narrower and leaves peach and rose to the records.
  { family: "danger", margin: 20, from: 4, to: 46 },
] as const;

/** How far blob B's hue sits from blob A's, in degrees, either way round. */
export const HUE_SPREAD = { min: 30, max: 50 } as const;

export type AvatarMesh = Readonly<{
  hueA: number;
  hueB: number;
  hueBase: number;
  ax: number;
  ay: number;
  bx: number;
  by: number;
  aSize: number;
  bSize: number;
}>;

/**
 * FNV-1a over code points: 32 bits, well spread for short keys, and identical in
 * every engine and locale. NFC first, so a composed and a decomposed "ü" are
 * one key.
 */
export function hashOf(key: string): number {
  let hash = 0x811c9dc5;
  for (const char of key.normalize("NFC")) {
    hash ^= char.codePointAt(0) ?? 0;
    hash = Math.imul(hash, 0x01000193);
  }
  return hash >>> 0;
}

// One hash, several parameters that must not move together: each draw re-mixes
// the hash with its own salt (murmur3's finaliser) and returns a fraction in [0, 1).
function draw(hash: number, salt: number): number {
  let x = (hash ^ Math.imul(salt + 1, 0x9e3779b9)) >>> 0;
  x = Math.imul(x ^ (x >>> 16), 0x85ebca6b);
  x = Math.imul(x ^ (x >>> 13), 0xc2b2ae35);
  return ((x ^ (x >>> 16)) >>> 0) / 2 ** 32;
}

/** The allowed arcs of the wheel, as [start, end] with end possibly past 360. */
export function allowedArcs(): readonly (readonly [number, number])[] {
  const bands = [...FORBIDDEN_HUES].sort((a, b) => a.from - b.from);
  return bands.map((band, i) => {
    const next = bands[(i + 1) % bands.length];
    const end = i + 1 < bands.length ? next.from : next.from + 360;
    return [band.to, end] as const;
  });
}

const wrap = (hue: number) => ((hue % 360) + 360) % 360;
const round1 = (n: number) => Math.round(n * 10) / 10;

/**
 * The three hues for a position in [0, 1) along the allowed wheel. Blob A is
 * spread evenly over every allowed degree; blob B turns the asked way if its
 * arc has the room, the other way if not, and in the narrow pink arc it takes
 * what room there is — so neither blob nor the base between them can step into
 * a forbidden band.
 */
export function huesAt(
  position: number,
  spread: number,
  clockwise: boolean,
): Readonly<{ hueA: number; hueB: number; hueBase: number }> {
  const arcs = allowedArcs();
  const total = arcs.reduce((sum, [from, to]) => sum + (to - from), 0);
  let along = position * total;
  let [start, end] = arcs[arcs.length - 1];
  let hueA = end;
  for (const [from, to] of arcs) {
    if (along < to - from) {
      [start, end, hueA] = [from, to, from + along];
      break;
    }
    along -= to - from;
  }
  const room = { up: end - hueA, down: hueA - start };
  const asked = clockwise ? "up" : "down";
  const other = clockwise ? "down" : "up";
  const way =
    room[asked] >= spread || room[asked] >= room[other] ? asked : other;
  const turn = Math.min(spread, room[way]) * (way === "up" ? 1 : -1);
  return {
    hueA: round1(wrap(hueA)),
    hueB: round1(wrap(hueA + turn)),
    hueBase: round1(wrap(hueA + turn / 2)),
  };
}

/** The whole composition for one record key. */
export function meshOf(key: string): AvatarMesh {
  const hash = hashOf(key);
  const spread =
    HUE_SPREAD.min + draw(hash, 1) * (HUE_SPREAD.max - HUE_SPREAD.min);
  const hues = huesAt(draw(hash, 0), spread, draw(hash, 2) < 0.5);
  // Blob A sits off-centre at an angle of its own; blob B roughly opposite it,
  // so every record's chip is lit from its own side.
  const angleA = draw(hash, 3) * 2 * Math.PI;
  const angleB = angleA + Math.PI + (draw(hash, 4) - 0.5) * (Math.PI / 1.5);
  const reach = 22 + draw(hash, 5) * 12;
  return {
    ...hues,
    ax: round1(50 + reach * Math.cos(angleA)),
    ay: round1(50 + reach * Math.sin(angleA)),
    bx: round1(50 + reach * Math.cos(angleB)),
    by: round1(50 + reach * Math.sin(angleB)),
    aSize: round1(62 + draw(hash, 6) * 24),
    bSize: round1(56 + draw(hash, 7) * 24),
  };
}

type MeshVars = CSSProperties & Record<`--${string}`, number>;

/** The mesh as the inline custom properties `.avatar-mesh` reads. */
export function meshStyle(mesh: AvatarMesh): MeshVars {
  return {
    "--avatar-hue-a": mesh.hueA,
    "--avatar-hue-b": mesh.hueB,
    "--avatar-hue-base": mesh.hueBase,
    "--avatar-ax": mesh.ax,
    "--avatar-ay": mesh.ay,
    "--avatar-bx": mesh.bx,
    "--avatar-by": mesh.by,
    "--avatar-a-size": mesh.aSize,
    "--avatar-b-size": mesh.bSize,
  };
}
