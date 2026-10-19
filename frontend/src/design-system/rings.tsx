// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "./rings.css";

/** How a ring's fill reads: the standing tones a record's 360 already uses. */
export type RingTone = "calm" | "warning" | "danger";

export type Ring = {
  key: string;
  // A share as its two halves, as `Meter` takes it, so the arc and the words
  // beside it cannot disagree. A zero max draws no arc.
  value: number;
  max: number;
  // Absent draws the track alone: a reading not taken is not a bad one.
  tone?: RingTone;
};

// The outermost radius, the step between rings and the stroke, in the
// figure's own units. Three rings fill the figure; a fourth would not fit.
const OUTER = 58;
const STEP = 14;
const STROKE = 10;
const SIZE = 2 * OUTER + STROKE;
export const MAX_RINGS = 3;

// A ring with no tone keeps the base class alone, and with it the neutral track.
function toneClass(base: string, tone?: RingTone): string {
  return tone ? `${base} rings-${tone}` : base;
}

/** A ring's tone as a dot, for the reading beside the figure that names it. */
export function RingDot({ tone }: Readonly<{ tone?: RingTone }>) {
  return <span className={toneClass("rings-dot", tone)} aria-hidden="true" />;
}

/**
 * Rings: up to three shares drawn as concentric arcs, the outermost first, so
 * a reader takes the shape of a record's readings in one glance. It is a
 * figure, not a label. `label` names the whole for a screen reader, and the
 * caller keeps the readings in text beside it, in the same order.
 */
export function Rings({
  rings,
  label,
}: Readonly<{ rings: readonly Ring[]; label: string }>) {
  const centre = SIZE / 2;
  return (
    <svg
      className="rings"
      viewBox={`0 0 ${SIZE} ${SIZE}`}
      role="img"
      aria-label={label}
    >
      {rings.slice(0, MAX_RINGS).map((ring, index) => {
        const radius = OUTER - index * STEP;
        const length = 2 * Math.PI * radius;
        const share = ring.max > 0 ? Math.min(ring.value / ring.max, 1) : 0;
        return (
          <g key={ring.key} className={toneClass("rings-ring", ring.tone)}>
            <circle
              className="rings-track"
              cx={centre}
              cy={centre}
              r={radius}
              strokeWidth={STROKE}
            />
            {share > 0 ? (
              <circle
                className="rings-arc"
                cx={centre}
                cy={centre}
                r={radius}
                strokeWidth={STROKE}
                strokeDasharray={length}
                strokeDashoffset={length * (1 - share)}
                transform={`rotate(-90 ${centre} ${centre})`}
              />
            ) : null}
          </g>
        );
      })}
    </svg>
  );
}
