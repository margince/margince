// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "./strengthmeter.css";

/** The bands a relationship's strength is read in, strongest first. */
export const STRENGTH_BANDS = ["strong", "moderate", "weak", "none"] as const;

export type StrengthBand = (typeof STRENGTH_BANDS)[number];

/**
 * StrengthMeter draws a relationship's band as three rising bars over its
 * word. The word is the fact and arrives translated; the bars are for a reader
 * scanning a column, so height and colour never carry the band alone.
 *
 * The standalone MCP App views draw the same markup through their own node
 * builder, because they carry no React; strengthmeter.css is the one sheet
 * both read.
 */
export function StrengthMeter({
  band,
  label,
}: Readonly<{ band: StrengthBand; label: string }>) {
  return (
    <span className="strength-meter" data-band={band}>
      <span className="strength-meter-bars" aria-hidden="true">
        <i />
        <i />
        <i />
      </span>
      {label}
    </span>
  );
}
