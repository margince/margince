// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "./stagepips.css";

/**
 * StagePips is how far along its pipeline a record stands. One short bar per
 * open stage, filled up to the one it is in, amber when it has stalled there.
 * The bars are decoration; `label`, the stage's name, carries the reading.
 */
export function StagePips({
  at,
  of,
  stalled = false,
  label,
}: Readonly<{
  // The stage's place among the open stages, from 1.
  at: number;
  of: number;
  stalled?: boolean;
  label: string;
}>) {
  const filled = stalled
    ? "stage-pip stage-pip-stalled"
    : "stage-pip stage-pip-on";
  return (
    <span className="stage-pips">
      <span className="stage-pips-track" aria-hidden="true">
        {Array.from({ length: of }, (_, index) => (
          <i
            // biome-ignore lint/suspicious/noArrayIndexKey: a pip is its place on the track, and nothing else
            key={index}
            className={index < at ? filled : "stage-pip"}
          />
        ))}
      </span>
      <span className="stage-pips-label">{label}</span>
    </span>
  );
}
