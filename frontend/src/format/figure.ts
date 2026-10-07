// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * Whether a figure reads as a floor. A FLOOR OF NONE IS NOT A FLOOR: "0+" says
 * "at least nothing", which is true of every number there is, so a bounded
 * read that found none is a reading of zero and the mark goes nowhere else.
 */
export function readsAsFloor(floor: boolean, value: number): boolean {
  return floor && value > 0;
}

/**
 * A figure the read behind it may have cut short: "200+" where it stopped at
 * its bound. A floor printed as a total is a wrong number rather than a
 * bounded one, and a reader has no way to tell the two apart.
 */
export function floorFigure(
  figure: string,
  floor: boolean,
  value: number,
): string {
  return readsAsFloor(floor, value) ? `${figure}+` : figure;
}
