// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** The reading ring's geometry, in the units of its 120 × 120 viewBox. */
export const RING = { size: 120, radius: 52, stroke: 9 } as const;

export const RING_CIRCUMFERENCE = 2 * Math.PI * RING.radius;

/** The dash that draws `fraction` of the ring from twelve o'clock; a fraction
 *  outside 0 to 1 is held to the ring. */
export function fillDash(fraction: number): string {
  const held = Math.min(Math.max(fraction, 0), 1);
  return `${round(held * RING_CIRCUMFERENCE)} ${round(RING_CIRCUMFERENCE)}`;
}

export type RingSegment = Readonly<{ dash: string; offset: number }>;

/**
 * Equal arcs round the ring, in order from twelve o'clock. `gapDegrees` of
 * track sits between each pair so that two arcs of one colour still read as two.
 */
export function segmentDashes(
  count: number,
  gapDegrees: number,
): RingSegment[] {
  if (count <= 0) {
    return [];
  }
  const share = RING_CIRCUMFERENCE / count;
  const gap = count === 1 ? 0 : (gapDegrees / 360) * RING_CIRCUMFERENCE;
  const drawn = round(Math.max(share - gap, 0));
  return Array.from({ length: count }, (_, index) => ({
    dash: `${drawn} ${round(RING_CIRCUMFERENCE - drawn)}`,
    offset: round(-index * share),
  }));
}

function round(length: number): number {
  return Math.round(length * 100) / 100;
}
