// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Five rules up the plot, each named on the right. With nothing read and no
// goal there is no scale to name, so only the base keeps its label.
export function ValueGrid({
  top,
  left,
  right,
  y,
  axisLabel,
}: Readonly<{
  top: number;
  left: number;
  right: number;
  y: (value: number) => number;
  axisLabel: (value: number) => string;
}>) {
  const named = top > 1;
  return (
    <>
      {[0, top / 4, top / 2, (top * 3) / 4, top].map((tick) => (
        <g key={tick}>
          <line
            className="trajectory-chart-grid"
            data-base={tick === 0 || undefined}
            x1={left}
            x2={right}
            y1={y(tick)}
            y2={y(tick)}
          />
          {(tick === 0 || named) && (
            <text
              className="trajectory-chart-axis"
              x={right + 10}
              y={y(tick) + 4}
            >
              {tick === 0 ? "0" : axisLabel(tick)}
            </text>
          )}
        </g>
      ))}
    </>
  );
}

// The pointer's week as a rule down the plot and a ring on the line.
export function Hair({
  x,
  y,
  span: [from, to],
}: Readonly<{ x: number; y: number; span: readonly [number, number] }>) {
  return (
    <g className="trajectory-chart-hair">
      <line x1={x} x2={x} y1={from} y2={to} />
      <circle cx={x} cy={y} r="4.5" />
    </g>
  );
}

// The week under the pointer: its date, its running total, and where an even
// pace to the goal would stand that day.
export function Readout({
  label,
  amount,
  at,
  left,
  top,
  pace,
  axisLabel,
}: Readonly<{
  label: string;
  amount: string;
  at: number;
  left: number;
  top: number;
  pace: { label: string; goal: number } | undefined;
  axisLabel: (value: number) => string;
}>) {
  return (
    <span
      className="trajectory-chart-readout"
      style={{ left: `${left}px`, top: `${top}px` }}
    >
      <span className="trajectory-chart-readout-label">{label}</span>
      <span className="trajectory-chart-readout-value">{amount}</span>
      {pace && (
        <span className="trajectory-chart-readout-label">
          {pace.label} {axisLabel(pace.goal * at)}
        </span>
      )}
    </span>
  );
}

export function nearest<Point extends Readonly<{ at: number }>>(
  points: readonly Point[],
  at: number,
): Point {
  return points.reduce((best, point) =>
    Math.abs(point.at - at) < Math.abs(best.at - at) ? point : best,
  );
}
