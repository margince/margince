// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * A pipeline's shape at a glance: its open stages left to right, each shaded by
 * how likely a deal standing there is to close won, and the ways out set apart
 * at the end. It is a READING of the ladder, never a control on it — nothing
 * here moves a record, which is what `StageLadder` is for.
 *
 * Two sizes of one drawing. The full strip names every step and its reading;
 * the compact one is a thin bar a list row carries so two pipelines can be told
 * apart before either is opened. Compact is decorative — the row beside it
 * states the count in words — so it is hidden from assistive technology rather
 * than read out as a string of unnamed shapes.
 */

import type { CSSProperties } from "react";
import "./stagestrip.css";

export type StripStep = Readonly<{
  key: string;
  name: string;
  // 0 to 100: how likely a deal standing here is to close won. It shades the
  // step, so the strip darkens toward the close.
  probability: number;
  // The same number as the reader sees it, formatted by the caller.
  reading: string;
  // A way out rather than one more rung. Outcome steps stand off at the end,
  // stacked, because they are alternatives to each other and not a sequence.
  outcome?: "won" | "lost";
}>;

type Shade = CSSProperties & Readonly<{ "--strip-weight": string }>;

function shade(probability: number): Shade {
  const weight = Math.min(100, Math.max(0, probability)) / 100;
  return { "--strip-weight": String(weight) };
}

export function StageStrip({
  label,
  steps,
  compact = false,
  empty,
}: Readonly<{
  // What the strip is a strip OF. Unused when compact, which is decorative.
  label: string;
  steps: readonly StripStep[];
  compact?: boolean;
  // What the open run says while it has no step, so an empty pipeline reads as
  // waiting rather than broken. Unused when compact.
  empty?: string;
}>) {
  const open = steps.filter((step) => step.outcome === undefined);
  const outcomes = steps.filter((step) => step.outcome !== undefined);
  if (compact) {
    return (
      <span className="stage-strip is-compact" aria-hidden="true">
        {open.map((step) => (
          <span
            key={step.key}
            className="stage-strip-step"
            style={shade(step.probability)}
          />
        ))}
        {open.length === 0 && <span className="stage-strip-step is-empty" />}
        {outcomes.map((step) => (
          <span
            key={step.key}
            className={`stage-strip-outcome is-${step.outcome}`}
          />
        ))}
      </span>
    );
  }
  return (
    <ol className="stage-strip" aria-label={label}>
      {open.map((step) => (
        <li
          key={step.key}
          className="stage-strip-step"
          style={shade(step.probability)}
        >
          <span className="stage-strip-name">{step.name}</span>
          <span className="stage-strip-reading t-num">{step.reading}</span>
        </li>
      ))}
      {open.length === 0 && (
        <li className="stage-strip-step is-empty">{empty}</li>
      )}
      {outcomes.length > 0 && (
        <li className="stage-strip-outcomes">
          <ol>
            {outcomes.map((step) => (
              <li
                key={step.key}
                className={`stage-strip-outcome is-${step.outcome}`}
              >
                <span className="stage-strip-name">{step.name}</span>
                <span className="stage-strip-reading t-num">
                  {step.reading}
                </span>
              </li>
            ))}
          </ol>
        </li>
      )}
    </ol>
  );
}
