// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "./progressbar.css";

// How far a running task has got. Not a native `<progress>`, whose track and
// fill are the browser's colours, which no token reaches. `valueText` is the
// words for the same number, so a reader of the role hears what the line under
// the bar states.
export function ProgressBar({
  fraction,
  label,
  valueText,
}: Readonly<{
  /** 0..1; out-of-range values are clamped. */
  fraction: number;
  label: string;
  valueText: string;
}>) {
  const percent = Math.round(Math.min(1, Math.max(0, fraction)) * 100);
  return (
    <div
      className="progressbar"
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={percent}
      aria-valuetext={valueText}
    >
      <span
        className="progressbar-fill"
        style={{ inlineSize: `${percent}%` }}
      />
    </div>
  );
}
