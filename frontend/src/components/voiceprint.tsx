// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import "./voiceprint.css";

// The waveform's shape is fixed: it is the mark of a voice, not a reading of
// one, so only its height (how much the voice has heard) and the ring around
// it carry a value.
const BARS = [
  18, 30, 46, 62, 40, 72, 90, 64, 38, 54, 82, 100, 76, 48, 60, 88, 70, 44, 58,
  36, 50, 28,
];

/**
 * A voice as a ring of progress around a waveform: the settings hero, the
 * onboarding collect and build scenes and a draft's voice match all draw it.
 * `fraction` fills the ring, `amplitude` scales the bars, and `children` take
 * the centre in place of the bars (a match percentage).
 */
export function Voiceprint({
  fraction,
  amplitude = 1,
  size = "large",
  busy,
  label,
  children,
}: Readonly<{
  fraction: number;
  amplitude?: number;
  size?: "small" | "medium" | "large";
  /** The bars move faster while a build runs. */
  busy?: boolean;
  /** The reading in words. Left out, the drawing is decoration beside a
   * reading the caller already writes out, and screen readers skip it. */
  label?: string;
  children?: ReactNode;
}>) {
  const filled = Math.min(1, Math.max(0, fraction));
  const radius = 46;
  const circumference = 2 * Math.PI * radius;
  return (
    <div
      className={`voiceprint voiceprint-${size}${busy ? " voiceprint-busy" : ""}`}
      {...(label === undefined
        ? { "aria-hidden": true }
        : { role: "img", "aria-label": label })}
    >
      <svg viewBox="0 0 100 100" aria-hidden="true">
        <circle className="voiceprint-track" cx="50" cy="50" r={radius} />
        <circle
          className="voiceprint-fill"
          cx="50"
          cy="50"
          r={radius}
          strokeDasharray={`${filled * circumference} ${circumference}`}
        />
      </svg>
      {children ? (
        <div className="voiceprint-centre">{children}</div>
      ) : (
        <div className="voiceprint-bars" aria-hidden="true">
          {BARS.map((height, index) => (
            <span
              // biome-ignore lint/suspicious/noArrayIndexKey: the bars are a fixed drawing; their order is their identity
              key={index}
              style={{
                height: `${Math.round(height * Math.max(0.15, amplitude))}%`,
                animationDelay: `${-(index * 0.13).toFixed(2)}s`,
              }}
            />
          ))}
        </div>
      )}
    </div>
  );
}
