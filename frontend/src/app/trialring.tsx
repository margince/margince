// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatNumber } from "../format/format";
import { useLocale } from "../i18n";
import type { TrialStanding } from "../lib/license-posture";
import "./trialring.css";

// The ring's radius in the orb box's 0..100 units. It clears the import ring the
// orb draws for itself, at every size the rail gives the ball.
const RADIUS = 38;
// The space between two day marks, in degrees, at most a third of a mark.
const GAP = 6;
// An arc that ends where it starts draws nothing, so the last one stops short.
const FULL_TURN = 359.99;

function pointAt(degrees: number): Readonly<{ x: number; y: number }> {
  const radians = (degrees * Math.PI) / 180;
  return {
    x: 50 + RADIUS * Math.sin(radians),
    y: 50 - RADIUS * Math.cos(radians),
  };
}

function arc(from: number, to: number): string {
  const start = pointAt(from);
  const end = pointAt(to);
  const large = to - from > 180 ? 1 : 0;
  return `M ${start.x} ${start.y} A ${RADIUS} ${RADIUS} 0 ${large} 1 ${end.x} ${end.y}`;
}

/**
 * The trial as a ring of day marks around the agent's orb. Days run clockwise
 * from twelve o'clock, so the lit marks still to come run out towards it, and a
 * dot marks today. The orb's own colours stay the agent's; the trial lives
 * outside them, so a failed import and a trial that ends soon never look alike.
 */
export function TrialRing({ standing }: Readonly<{ standing: TrialStanding }>) {
  const { locale } = useLocale();
  const total = standing.totalDays;
  const step = 360 / total;
  const gap = Math.min(GAP, step / 3);
  const used = total - standing.daysLeft;
  const today = pointAt(used * step);
  return (
    <span
      className="artrialring"
      data-closing={standing.closing ? "" : undefined}
      aria-hidden="true"
    >
      <svg viewBox="0 0 100 100" focusable="false" aria-hidden="true">
        <g className="artrialring-marks">
          {Array.from({ length: total }, (_, index) => index).map((day) => (
            <path
              key={day}
              className={day < used ? "used" : "left"}
              d={arc(day * step + gap / 2, (day + 1) * step - gap / 2)}
            />
          ))}
        </g>
        {/* The folded rail has no room for a mark per day: one arc says the same. */}
        <g className="artrialring-arc">
          <circle className="used" cx="50" cy="50" r={RADIUS} />
          {standing.daysLeft > 0 && (
            <path className="left" d={arc(used * step, FULL_TURN)} />
          )}
        </g>
        <circle
          className="artrialring-today"
          cx={today.x}
          cy={today.y}
          r="2.4"
        />
      </svg>
      {standing.closing && (
        <span className="artrialring-count">
          {formatNumber(standing.daysLeft, locale)}
        </span>
      )}
    </span>
  );
}
