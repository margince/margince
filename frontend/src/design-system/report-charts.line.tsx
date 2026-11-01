// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useEffect, useId, useRef, useState } from "react";
import { Button } from "./atoms";
import type { ChartProps, ChartReading } from "./report-charts.model";
import { niceAxis, readingScale } from "./report-charts.model";
import { ReadingTable } from "./report-charts.table";
import "./report-charts.css";

function lineSegments(
  readings: readonly ChartReading[],
  field: "value" | "comparison",
  width: number,
  y: (value: number) => number,
) {
  const lines: string[] = [];
  let points: string[] = [];
  readings.forEach((reading, index) => {
    const value = reading[field];
    if (value == null) {
      if (points.length) lines.push(points.join(" "));
      points = [];
    } else points.push(`${chartX(index, readings.length, width)},${y(value)}`);
  });
  if (points.length) lines.push(points.join(" "));
  return lines;
}

function chartX(index: number, count: number, width: number) {
  return 56 + (index / Math.max(1, count - 1)) * (width - 76);
}
// The plot's height in viewBox units, mirrored by `.report-chart-line`.
const LINE_HEIGHT = { default: 238, compact: 160 } as const;

function chartY(value: number, scale: number, height: number) {
  return height - 40 - (value / scale) * (height - 64);
}

export function CumulativeChart({
  readings,
  label,
  dataLabel,
  valueLabel,
  comparisonLabel,
  reference,
  axisLabel,
  onSelect,
  size = "default",
}: ChartProps &
  Readonly<{
    comparisonLabel?: string;
    reference?: Readonly<{ value: number; label: string; amount: string }>;
    axisLabel: (value: number) => string;
    /** `compact` is a shorter plot, for a chart beside a lead figure. */
    size?: keyof typeof LINE_HEIGHT;
  }>) {
  const height = LINE_HEIGHT[size];
  const title = useId();
  const plot = useRef<HTMLDivElement>(null);
  const width = usePlotWidth(plot);
  const [active, setActive] = useState<string | null>(null);
  const axis = niceAxis(
    readingScale(readings, reference?.value),
    size === "compact" ? 2 : 4,
  );
  const y = (value: number) => chartY(value, axis.top, height);
  const x = (index: number) => chartX(index, readings.length, width);
  const selectedIndex = readings.findIndex((reading) => reading.key === active);
  const selected = readings[selectedIndex];
  const lastIndex = readings.reduce(
    (found, reading, index) => (reading.value == null ? found : index),
    -1,
  );
  const last = readings[lastIndex];
  const labels = new Set([
    0,
    Math.floor((readings.length - 1) / 2),
    readings.length - 1,
  ]);
  // The pointer reads the nearest day, so the whole plot answers a hover and
  // not only the dots.
  const follow = (clientX: number) => {
    const box = plot.current?.getBoundingClientRect();
    if (!box?.width || readings.length === 0) return;
    const at = ((clientX - box.left) / box.width) * width;
    const span = Math.max(1, readings.length - 1);
    const index = Math.round(((at - 56) / (width - 76)) * span);
    const reading = readings[Math.min(span, Math.max(0, index))];
    if (reading) setActive(reading.key);
  };
  return (
    <figure className="report-chart" data-size={size} aria-labelledby={title}>
      <figcaption id={title} className="sr-only">
        {label}
      </figcaption>
      <div className="report-chart-legend">
        <span className="report-chart-key">{valueLabel}</span>
        {comparisonLabel && (
          <span className="report-chart-key report-chart-key-comparison">
            {comparisonLabel}
          </span>
        )}
        {reference && (
          <span className="report-chart-key report-chart-key-target">
            {reference.label} · {reference.amount}
          </span>
        )}
      </div>
      <div
        className="report-chart-line"
        data-size={size}
        ref={plot}
        onPointerMove={(event) => follow(event.clientX)}
        onPointerLeave={() => setActive(null)}
      >
        <svg
          viewBox={`0 0 ${width} ${height}`}
          aria-hidden="true"
          preserveAspectRatio="none"
        >
          {axis.ticks.map((tick) => (
            <g key={tick}>
              <line
                className="report-chart-grid"
                x1="56"
                x2={width - 20}
                y1={y(tick)}
                y2={y(tick)}
              />
              <text
                className="report-chart-axis"
                x="48"
                y={y(tick) + 4}
                textAnchor="end"
              >
                {axisLabel(tick)}
              </text>
            </g>
          ))}
          {reference && (
            <line
              className="report-chart-reference"
              x1="56"
              x2={width - 20}
              y1={y(reference.value)}
              y2={y(reference.value)}
            />
          )}
          <g className="report-chart-series">
            {areaSegments(readings, x, y, y(0)).map((path) => (
              <path key={path} className="report-chart-area" d={path} />
            ))}
            {lineSegments(readings, "comparison", width, y).map((points) => (
              <polyline
                key={points}
                className="report-chart-comparison"
                points={points}
              />
            ))}
            {lineSegments(readings, "value", width, y).map((points) => (
              <polyline
                key={points}
                className="report-chart-actual"
                points={points}
              />
            ))}
          </g>
          {selected && (
            <line
              className="report-chart-crosshair"
              x1={x(selectedIndex)}
              x2={x(selectedIndex)}
              y1={y(axis.top)}
              y2={y(0)}
            />
          )}
          {last?.value != null && !selected && (
            <text
              className="report-chart-end"
              x={x(lastIndex)}
              y={y(last.value) - 12}
              textAnchor="end"
            >
              {last.amount}
            </text>
          )}
          {readings.map(
            (reading, index) =>
              labels.has(index) && (
                <text
                  key={reading.key}
                  className="report-chart-axis"
                  x={x(index)}
                  y={height - 13}
                  textAnchor={
                    index === 0
                      ? "start"
                      : index === readings.length - 1
                        ? "end"
                        : "middle"
                  }
                >
                  {reading.label}
                </text>
              ),
          )}
        </svg>
        {readings.map(
          (reading, index) =>
            reading.value != null && (
              <Button
                key={reading.key}
                variant="link"
                className="report-chart-point"
                data-active={reading.key === active || undefined}
                data-last={index === lastIndex || undefined}
                style={{
                  left: `${(x(index) / width) * 100}%`,
                  top: `${(y(reading.value) / height) * 100}%`,
                }}
                aria-label={`${reading.label}: ${reading.amount}`}
                onFocus={() => setActive(reading.key)}
                onBlur={() => setActive(null)}
                onClick={() => onSelect?.(reading.key)}
                disabled={!onSelect}
              >
                <span className="report-chart-dot" />
              </Button>
            ),
        )}
        {selected?.value != null && (
          <div
            className="report-chart-tip"
            aria-hidden="true"
            data-side={selectedIndex > readings.length / 2 ? "start" : "end"}
            style={{
              left: `${(x(selectedIndex) / width) * 100}%`,
              top: `${(y(selected.value) / height) * 100}%`,
            }}
          >
            <span className="report-chart-tip-label">{selected.label}</span>
            <span className="report-chart-tip-value">{selected.amount}</span>
            {comparisonLabel && selected.comparisonAmount && (
              <span className="report-chart-tip-label">
                {comparisonLabel}: {selected.comparisonAmount}
              </span>
            )}
          </div>
        )}
      </div>
      <div className="report-chart-focus sr-only" aria-live="polite">
        {selected ? `${selected.label} · ${selected.amount}` : ""}
      </div>
      <ReadingTable
        readings={readings}
        label={label}
        dataLabel={dataLabel}
        valueLabel={valueLabel}
        secondaryLabel={comparisonLabel}
        secondary="comparisonAmount"
        onSelect={onSelect}
      />
    </figure>
  );
}

// The plot's drawn width, followed as its column resizes, so a line keeps
// its own stroke and labels instead of stretching them.
function usePlotWidth(plot: RefObject<HTMLDivElement | null>) {
  const [width, setWidth] = useState(680);
  useEffect(() => {
    const node = plot.current;
    if (!node || typeof ResizeObserver === "undefined") return;
    const measure = () => setWidth(Math.max(200, node.clientWidth));
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(node);
    return () => observer.disconnect();
  }, [plot]);
  return width;
}

// The wash under each unbroken run of the actual line, down to zero. A gap
// in the readings stays a gap in the wash.
function areaSegments(
  readings: readonly ChartReading[],
  x: (index: number) => number,
  y: (value: number) => number,
  base: number,
) {
  const areas: string[] = [];
  let run: [number, number][] = [];
  const close = () => {
    const first = run[0];
    const last = run.at(-1);
    if (run.length > 1 && first && last) {
      const line = run.map(([px, py]) => `L${px},${py}`).join(" ");
      areas.push(`M${first[0]},${base} ${line} L${last[0]},${base} Z`);
    }
    run = [];
  };
  readings.forEach((reading, index) => {
    if (reading.value == null) close();
    else run.push([x(index), y(reading.value)]);
  });
  close();
  return areas;
}
