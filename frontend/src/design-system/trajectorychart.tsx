// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useId, useRef, useState } from "react";
import { Badge, Button } from "./atoms";
import { ReadingTable } from "./report-charts";
import type { ChartReading } from "./report-charts.model";
import "./trajectorychart.css";

/** A running total read through one moment of the period, `at` from 0 to 1. */
export type TrajectoryPoint = ChartReading &
  Readonly<{ value: number; at: number }>;

export type TrajectoryChartProps = Readonly<{
  label: string;
  dataLabel: string;
  valueLabel: string;
  // Read so far, in order. The last one is where the line stops: today.
  points: readonly TrajectoryPoint[];
  // Where the line stops, named on the plot ("Today · 31 Oct").
  nowLabel: string;
  // The period's divisions on the time axis, each at its start.
  ticks: readonly Readonly<{ at: number; label: string }>[];
  axisLabel: (value: number) => string;
  target?: Readonly<{ value: number; label: string }>;
  // Where the period is heading on what is already committed, drawn dashed
  // from today to the period's end.
  projection?: Readonly<{ value: number; label: string }>;
  // The modelled finish, a band from today that widens to the period's end.
  range?: Readonly<{ low: number; high: number; label: string }>;
  // What the projection leaves short of the target, as words under it.
  shortfall?: string;
  onSelect?: (key: string) => void;
}>;

const HEIGHT = 190;
const LEFT = 44;
const RIGHT = 12;
const TOP = 14;
const BOTTOM = 26;

/**
 * A period's running total against where it can finish. The line is what was
 * read so far, and the committed projection runs dashed to the period's end.
 * The modelled range is a band that widens from today; the target is one rule
 * across. Labels sit inside the plot, so the line runs to the card's edge.
 */
export function TrajectoryChart({
  label,
  dataLabel,
  valueLabel,
  points,
  nowLabel,
  ticks,
  axisLabel,
  target,
  projection,
  range,
  shortfall,
  onSelect,
}: TrajectoryChartProps) {
  const title = useId();
  const plot = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(720);
  const [active, setActive] = useState<string | null>(null);
  useEffect(() => {
    const node = plot.current;
    if (!node || typeof ResizeObserver === "undefined") return;
    // The plot's own width, unclamped: the marks are placed in pixels over
    // the drawing, so the drawing must be drawn at the width it shows at.
    const measure = () =>
      setWidth(Math.max(LEFT + RIGHT + 1, node.clientWidth));
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(node);
    return () => observer.disconnect();
  }, []);
  const now = points.at(-1);
  if (!now) return null;
  const top = niceTop(
    Math.max(
      now.value,
      target?.value ?? 0,
      projection?.value ?? 0,
      range?.high ?? 0,
    ),
  );
  const right = width - RIGHT;
  const x = (at: number) => LEFT + at * (right - LEFT);
  const y = (value: number) =>
    HEIGHT - BOTTOM - (value / top) * (HEIGHT - BOTTOM - TOP);
  const line = smoothPath([
    [x(0), y(0)],
    ...points.map((point): Pair => [x(point.at), y(point.value)]),
  ]);
  const nowX = x(now.at);
  const nowY = y(now.value);
  const narrow = width < 480;
  const selected = points.find((point) => point.key === active);
  return (
    <figure className="report-chart trajectory-chart" aria-labelledby={title}>
      <figcaption id={title} className="sr-only">
        {label}
      </figcaption>
      <div className="trajectory-chart-plot" ref={plot}>
        <svg viewBox={`0 0 ${width} ${HEIGHT}`} aria-hidden="true">
          <defs>
            <linearGradient id={`${title}-area`} x1="0" x2="0" y1="0" y2="1">
              <stop offset="0" className="trajectory-chart-area-top" />
              <stop offset="1" className="trajectory-chart-area-foot" />
            </linearGradient>
          </defs>
          {[0, top / 2, top].map((tick) => (
            <g key={tick}>
              <line
                className="trajectory-chart-grid"
                data-base={tick === 0 || undefined}
                x1={LEFT}
                x2={right}
                y1={y(tick)}
                y2={y(tick)}
              />
              <text
                className="trajectory-chart-axis"
                x={LEFT - 8}
                y={y(tick) + 4}
                textAnchor="end"
              >
                {tick === 0 ? "0" : axisLabel(tick)}
              </text>
            </g>
          ))}
          {ticks.map((tick) => (
            <g key={tick.label}>
              <line
                className="trajectory-chart-tick"
                x1={x(tick.at)}
                x2={x(tick.at)}
                y1={HEIGHT - BOTTOM}
                y2={HEIGHT - BOTTOM + 5}
              />
              <text
                className="trajectory-chart-axis"
                x={x(tick.at) + 6}
                y={HEIGHT - 6}
              >
                {tick.label}
              </text>
            </g>
          ))}
          <rect
            className="trajectory-chart-ahead"
            x={nowX}
            y={TOP - 6}
            width={Math.max(0, right - nowX)}
            height={HEIGHT - BOTTOM - TOP + 6}
          />
          <AheadPaths
            scale={{ x, y }}
            now={now}
            range={range}
            projection={projection}
          />
          {target && (
            <line
              className="trajectory-chart-target"
              x1={LEFT}
              x2={right}
              y1={y(target.value)}
              y2={y(target.value)}
            />
          )}
          <path
            className="trajectory-chart-area"
            d={`${line} L${nowX},${y(0)} L${x(0)},${y(0)} Z`}
            fill={`url(#${CSS.escape(`${title}-area`)})`}
          />
          <path className="trajectory-chart-line" d={line} pathLength={1} />
          <line
            className="trajectory-chart-now"
            x1={nowX}
            x2={nowX}
            y1={TOP - 6}
            y2={HEIGHT - BOTTOM}
          />
          <text className="trajectory-chart-now-label" x={nowX + 8} y={TOP + 6}>
            {nowLabel}
          </text>
          <FinishMarks
            scale={{ x, y }}
            narrow={narrow}
            now={now}
            target={target}
            range={range}
            projection={projection}
            shortfall={shortfall}
          />
          <circle
            className="trajectory-chart-now-dot"
            cx={nowX}
            cy={nowY}
            r="6"
          />
        </svg>
        {target && (
          <span
            className="trajectory-chart-target-name"
            data-narrow={narrow || undefined}
            style={{ top: `${y(target.value)}px`, left: `${LEFT + 6}px` }}
          >
            <Badge variant="primary">{target.label}</Badge>
          </span>
        )}
        {points.map((point) => (
          <Button
            key={point.key}
            variant="link"
            className="report-chart-point"
            style={{ left: `${x(point.at)}px`, top: `${y(point.value)}px` }}
            aria-label={`${point.label}: ${point.amount}`}
            onFocus={() => setActive(point.key)}
            onPointerEnter={() => setActive(point.key)}
            onPointerLeave={() => setActive(null)}
            onClick={() => onSelect?.(point.key)}
            disabled={!onSelect}
          >
            <span className="report-chart-dot" />
          </Button>
        ))}
      </div>
      <div className="report-chart-focus" aria-live="polite">
        {selected ? `${selected.label} · ${selected.amount}` : ""}
      </div>
      <ReadingTable
        readings={points}
        label={label}
        dataLabel={dataLabel}
        valueLabel={valueLabel}
        onSelect={onSelect}
      />
    </figure>
  );
}

type Pair = readonly [number, number];
type Scale = Readonly<{
  x: (at: number) => number;
  y: (value: number) => number;
}>;
type Ahead = Pick<
  TrajectoryChartProps,
  "target" | "projection" | "range" | "shortfall"
> &
  Readonly<{ scale: Scale; now: TrajectoryPoint }>;

// The modelled band and the committed projection, from today to the end.
function AheadPaths({ scale: { x, y }, now, range, projection }: Ahead) {
  const from: Pair = [x(now.at), y(now.value)];
  const ahead = (share: number) => x(now.at + (1 - now.at) * share);
  return (
    <>
      {range && (
        <path
          className="trajectory-chart-range"
          d={rangePath(from[0], from[1], x(1), y(range.high), y(range.low))}
        />
      )}
      {projection && (
        <path
          className="trajectory-chart-projection"
          d={`M${from[0]},${from[1]} C${ahead(0.4)},${y(now.value + (projection.value - now.value) * 0.35)} ${ahead(0.7)},${y(now.value + (projection.value - now.value) * 0.94)} ${x(1)},${y(projection.value)}`}
        />
      )}
    </>
  );
}

// What the period's end holds: the band's name, the projected finish, and
// the gap to the target when the projection falls short of it.
function FinishMarks({
  scale: { x, y },
  narrow,
  now,
  target,
  range,
  projection,
  shortfall,
}: Ahead & Readonly<{ narrow: boolean }>) {
  const endX = x(1);
  // The words sit under the band's lower edge, clear of the line and the cone.
  const under = y(range ? range.low : (projection?.value ?? 0));
  const short = projection && target && projection.value < target.value;
  return (
    <>
      {/* On a phone the band's name would sit on today's; the plate above
          already names the modelled finish. */}
      {range && !narrow && (
        <text
          className="trajectory-chart-range-label"
          x={x(now.at + (1 - now.at) * 0.6)}
          y={Math.max(TOP + 6, y(range.high) - 4)}
          textAnchor="middle"
        >
          {range.label}
        </text>
      )}
      {short && (
        <rect
          className="trajectory-chart-gap"
          x={endX - 3}
          y={y(target.value)}
          width="6"
          height={y(projection.value) - y(target.value)}
          rx="3"
        />
      )}
      {projection && (
        <>
          <circle
            className="trajectory-chart-end"
            cx={endX}
            cy={y(projection.value)}
            r="4"
          />
          <text
            className="trajectory-chart-end-label"
            x={endX}
            y={under + 19}
            textAnchor="end"
          >
            {projection.label}
          </text>
        </>
      )}
      {shortfall && (
        <text
          className="trajectory-chart-shortfall"
          x={endX}
          y={under + 36}
          textAnchor="end"
        >
          {shortfall}
        </text>
      )}
    </>
  );
}

// The axis top: the largest value drawn, raised to a round step so the two
// grid lines above zero read as round amounts.
function niceTop(value: number): number {
  if (value <= 0) return 1;
  const magnitude = 10 ** Math.floor(Math.log10(value));
  const step = [1, 2, 2.5, 5, 10].find((m) => m * magnitude >= value * 1.04);
  return (step ?? 10) * magnitude;
}

// A curve through every point (Catmull-Rom as cubic Béziers), so a weekly
// running total reads as one movement rather than a row of corners.
function smoothPath(points: readonly Pair[]): string {
  const [first] = points;
  if (!first) return "";
  let path = `M${first[0]},${first[1]}`;
  for (let index = 1; index < points.length; index++) {
    const p0 = points[index - 2] ?? points[index - 1] ?? first;
    const p1 = points[index - 1] ?? first;
    const p2 = points[index] ?? p1;
    const p3 = points[index + 1] ?? p2;
    const c1 = [p1[0] + (p2[0] - p0[0]) / 6, p1[1] + (p2[1] - p0[1]) / 6];
    const c2 = [p2[0] - (p3[0] - p1[0]) / 6, p2[1] - (p3[1] - p1[1]) / 6];
    path += ` C${c1[0]},${c1[1]} ${c2[0]},${c2[1]} ${p2[0]},${p2[1]}`;
  }
  return path;
}

// The band from today's point that opens to the range at the period's end.
function rangePath(
  fromX: number,
  fromY: number,
  toX: number,
  highY: number,
  lowY: number,
): string {
  const span = toX - fromX;
  return [
    `M${fromX},${fromY}`,
    `C${fromX + span * 0.55},${fromY + (highY - fromY) * 0.2} ${fromX + span * 0.8},${highY + (fromY - highY) * 0.08} ${toX},${highY}`,
    `L${toX},${lowY}`,
    `C${fromX + span * 0.8},${lowY + (fromY - lowY) * 0.05} ${fromX + span * 0.45},${fromY + (lowY - fromY) * 0.1} ${fromX},${fromY}`,
    "Z",
  ].join(" ");
}
