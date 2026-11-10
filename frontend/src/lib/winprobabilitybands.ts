// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../i18n/en";
import { encodeRange, parsePercentRange } from "./dealranges";

export type WinProbabilityBand = Readonly<{
  key: string;
  min: number;
  max: number;
  label: MessageKey;
}>;

// The report engine's `win_probability_band` values and the whole percents
// each one holds, in the order the tiles read. The server's CASE is the owner;
// a band it adds is absent here until this list learns it.
export const WIN_PROBABILITY_BANDS: readonly WinProbabilityBand[] = [
  {
    key: "90_plus",
    min: 90,
    max: 100,
    label: "deal.winProbability.band.90_plus",
  },
  { key: "60_89", min: 60, max: 89, label: "deal.winProbability.band.60_89" },
  { key: "30_59", min: 30, max: 59, label: "deal.winProbability.band.30_59" },
  { key: "1_29", min: 1, max: 29, label: "deal.winProbability.band.1_29" },
  { key: "zero", min: 0, max: 0, label: "deal.winProbability.band.zero" },
];

/** The deals list's probability dial narrowed to one band. */
export function bandDialValue(band: WinProbabilityBand): string {
  return encodeRange(String(band.min), String(band.max));
}

export function bandOfKey(key: string): WinProbabilityBand | undefined {
  return WIN_PROBABILITY_BANDS.find((band) => band.key === key);
}

/** The band whose bounds a probability dial matches, or undefined for any other range. */
export function bandOfDial(
  value: string | undefined,
): WinProbabilityBand | undefined {
  const range = parsePercentRange(value);
  return WIN_PROBABILITY_BANDS.find(
    (band) => band.min === range?.min && band.max === range?.max,
  );
}
