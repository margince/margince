// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type CSSProperties, useEffect, useId, useRef, useState } from "react";
import { Button, Disclosure, TableScroll } from "./atoms";
import "./report-charts.css";

export type ChartReading = Readonly<{
  key: string;
  label: string;
  value: number | null;
  amount: string;
  comparison?: number | null;
  comparisonAmount?: string;
  target?: number | null;
  targetAmount?: string;
  upper?: number | null;
  upperAmount?: string;
}>;

type ChartProps = Readonly<{
  readings: readonly ChartReading[];
  label: string;
  dataLabel: string;
  valueLabel: string;
  onSelect?: (key: string) => void;
}>;

function readingScale(readings: readonly ChartReading[], reference = 0) {
  return Math.max(
    1,
    reference,
    ...readings.flatMap((reading) => [
      reading.value ?? 0,
      reading.comparison ?? 0,
      reading.target ?? 0,
      reading.upper ?? 0,
    ]),
  );
}

function ReadingTable({
  readings,
  label,
  dataLabel,
  valueLabel,
  secondaryLabel,
  secondary,
  onSelect,
}: ChartProps &
  Readonly<{
    secondaryLabel?: string;
    secondary?: "comparisonAmount" | "targetAmount" | "upperAmount";
  }>) {
  return (
    <Disclosure className="report-chart-data" summary={dataLabel}>
      <TableScroll className="report-chart-table-scroll" label={label}>
        <table>
          <caption className="sr-only">{label}</caption>
          <thead>
            <tr>
              <th scope="col">{label}</th>
              <th scope="col">{valueLabel}</th>
              {secondaryLabel && <th scope="col">{secondaryLabel}</th>}
            </tr>
          </thead>
          <tbody>
            {readings.map((reading) => (
              <tr key={reading.key}>
                <th scope="row">
                  {onSelect ? (
                    <Button
                      variant="link"
                      onClick={() => onSelect(reading.key)}
                    >
                      {reading.label}
                    </Button>
                  ) : (
                    reading.label
                  )}
                </th>
                <td>{reading.amount}</td>
                {secondaryLabel && (
                  <td>{secondary ? reading[secondary] : null}</td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      </TableScroll>
    </Disclosure>
  );
}

function lineSegments(
  readings: readonly ChartReading[],
  field: "value" | "comparison",
  scale: number,
  width: number,
) {
  const lines: string[] = [];
  let points: string[] = [];
  readings.forEach((reading, index) => {
    const value = reading[field];
    if (value == null) {
      if (points.length) lines.push(points.join(" "));
      points = [];
    } else
      points.push(
        `${chartX(index, readings.length, width)},${chartY(value, scale)}`,
      );
  });
  if (points.length) lines.push(points.join(" "));
  return lines;
}

function chartX(index: number, count: number, width: number) {
  return 56 + (index / Math.max(1, count - 1)) * (width - 76);
}
function chartY(value: number, scale: number) {
  return 198 - (value / scale) * 174;
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
}: ChartProps &
  Readonly<{
    comparisonLabel: string;
    reference?: Readonly<{ value: number; label: string; amount: string }>;
    axisLabel: (value: number) => string;
  }>) {
  const title = useId();
  const plot = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(680);
  useEffect(() => {
    const node = plot.current;
    if (!node || typeof ResizeObserver === "undefined") return;
    const measure = () => setWidth(Math.max(200, node.clientWidth));
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(node);
    return () => observer.disconnect();
  }, []);
  const [active, setActive] = useState<string | null>(null);
  const scale = readingScale(readings, reference?.value) * 1.08;
  const selected = readings.find((reading) => reading.key === active);
  const ticks = [0, scale / 2, scale];
  const labels = new Set([
    0,
    Math.floor((readings.length - 1) / 2),
    readings.length - 1,
  ]);
  return (
    <figure className="report-chart" aria-labelledby={title}>
      <figcaption id={title} className="sr-only">
        {label}
      </figcaption>
      <div className="report-chart-legend">
        <span className="report-chart-key">{valueLabel}</span>
        <span className="report-chart-key report-chart-key-comparison">
          {comparisonLabel}
        </span>
        {reference && (
          <span className="report-chart-key report-chart-key-target">
            {reference.label} · {reference.amount}
          </span>
        )}
      </div>
      <div className="report-chart-line" ref={plot}>
        <svg
          viewBox={`0 0 ${width} 238`}
          aria-hidden="true"
          preserveAspectRatio="none"
        >
          {ticks.map((tick) => (
            <g key={tick}>
              <line
                className="report-chart-grid"
                x1="56"
                x2={width - 20}
                y1={chartY(tick, scale)}
                y2={chartY(tick, scale)}
              />
              <text
                className="report-chart-axis"
                x="48"
                y={chartY(tick, scale) + 4}
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
              y1={chartY(reference.value, scale)}
              y2={chartY(reference.value, scale)}
            />
          )}
          {lineSegments(readings, "comparison", scale, width).map((points) => (
            <polyline
              key={points}
              className="report-chart-comparison"
              points={points}
            />
          ))}
          {lineSegments(readings, "value", scale, width).map((points) => (
            <polyline
              key={points}
              className="report-chart-actual"
              points={points}
            />
          ))}
          {readings.map(
            (reading, index) =>
              labels.has(index) && (
                <text
                  key={reading.key}
                  className="report-chart-axis"
                  x={chartX(index, readings.length, width)}
                  y="225"
                  textAnchor="middle"
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
                style={{
                  left: `${(chartX(index, readings.length, width) / width) * 100}%`,
                  top: `${(chartY(reading.value, scale) / 238) * 100}%`,
                }}
                aria-label={`${reading.label}: ${reading.amount}`}
                onFocus={() => setActive(reading.key)}
                onPointerEnter={() => setActive(reading.key)}
                onClick={() => onSelect?.(reading.key)}
                disabled={!onSelect}
              >
                <span className="report-chart-dot" />
              </Button>
            ),
        )}
      </div>
      <div className="report-chart-focus" aria-live="polite">
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

export function BulletChart({
  readings,
  label,
  dataLabel,
  valueLabel,
  targetLabel,
  onSelect,
}: ChartProps & Readonly<{ targetLabel: string }>) {
  const scale = readingScale(readings) * 1.08;
  return (
    <figure className="report-chart">
      <figcaption className="sr-only">{label}</figcaption>
      <div className="report-chart-legend">
        <span className="report-chart-key">{valueLabel}</span>
        <span className="report-chart-key report-chart-key-target">
          {targetLabel}
        </span>
      </div>
      <ul className="report-chart-rows">
        {readings.map((reading) => (
          <li key={reading.key}>
            <Button
              variant="link"
              className="report-chart-row"
              onClick={() => onSelect?.(reading.key)}
              disabled={!onSelect}
              style={markGeometry(
                (Math.max(0, reading.value ?? 0) / scale) * 100,
              )}
              aria-label={`${reading.label}: ${reading.amount}; ${targetLabel}: ${reading.targetAmount ?? "—"}`}
            >
              <span className="report-chart-row-name">{reading.label}</span>
              <span className="report-chart-bullet" aria-hidden="true">
                {reading.value != null && (
                  <span className="report-chart-bullet-fill" />
                )}
                {reading.target != null && (
                  <span
                    className="report-chart-bullet-target"
                    style={{ left: `${(reading.target / scale) * 100}%` }}
                  />
                )}
              </span>
              <span className="report-chart-row-amount">
                {reading.amount}
                <small>{reading.targetAmount}</small>
              </span>
            </Button>
          </li>
        ))}
      </ul>
      <ReadingTable
        readings={readings}
        label={label}
        dataLabel={dataLabel}
        valueLabel={valueLabel}
        secondaryLabel={targetLabel}
        secondary="targetAmount"
        onSelect={onSelect}
      />
    </figure>
  );
}

export function RangeChart({
  readings,
  label,
  dataLabel,
  valueLabel,
  upperLabel,
  onSelect,
}: ChartProps & Readonly<{ upperLabel: string }>) {
  const scale = readingScale(readings) * 1.08;
  return (
    <figure className="report-chart">
      <figcaption className="sr-only">{label}</figcaption>
      <div className="report-chart-legend">
        <span className="report-chart-key report-chart-key-dot">
          {valueLabel}
        </span>
        <span className="report-chart-key report-chart-key-range">
          {upperLabel}
        </span>
      </div>
      <ul className="report-chart-rows">
        {readings.map((reading) => (
          <li key={reading.key}>
            <Button
              variant="link"
              className="report-chart-row"
              onClick={() => onSelect?.(reading.key)}
              disabled={!onSelect}
              style={markGeometry(
                (Math.max(0, (reading.upper ?? 0) - (reading.value ?? 0)) /
                  scale) *
                  100,
                ((reading.value ?? 0) / scale) * 100,
              )}
              aria-label={`${reading.label}: ${valueLabel} ${reading.amount}; ${upperLabel} ${reading.upperAmount ?? "—"}`}
            >
              <span className="report-chart-row-name">{reading.label}</span>
              <span className="report-chart-range" aria-hidden="true">
                {reading.value != null && reading.upper != null && (
                  <>
                    <span className="report-chart-range-band" />
                    <span
                      className="report-chart-range-dot"
                      style={{ left: `${(reading.value / scale) * 100}%` }}
                    />
                    <span
                      className="report-chart-range-end"
                      style={{ left: `${(reading.upper / scale) * 100}%` }}
                    />
                  </>
                )}
              </span>
              <span className="report-chart-row-amount">
                {reading.amount}
                <small>{reading.upperAmount}</small>
              </span>
            </Button>
          </li>
        ))}
      </ul>
      <ReadingTable
        readings={readings}
        label={label}
        dataLabel={dataLabel}
        valueLabel={valueLabel}
        secondaryLabel={upperLabel}
        secondary="upperAmount"
        onSelect={onSelect}
      />
    </figure>
  );
}

export function GroupedBars({
  readings,
  label,
  dataLabel,
  valueLabel,
  comparisonLabel,
  onSelect,
}: ChartProps & Readonly<{ comparisonLabel: string }>) {
  const scale = readingScale(readings) * 1.15;
  return (
    <figure className="report-chart">
      <figcaption className="sr-only">{label}</figcaption>
      <div className="report-chart-legend">
        <span className="report-chart-key">{valueLabel}</span>
        <span className="report-chart-key report-chart-key-comparison">
          {comparisonLabel}
        </span>
      </div>
      <div className="report-chart-columns">
        {readings.map((reading) => (
          <div key={reading.key} className="report-chart-column-group">
            <div className="report-chart-column-pair">
              <Button
                variant="link"
                className="report-chart-column"
                style={{
                  height: `${(Math.max(0, reading.value ?? 0) / scale) * 100}%`,
                }}
                aria-label={`${reading.label}, ${valueLabel}: ${reading.amount}`}
                onClick={() => onSelect?.(reading.key)}
                disabled={!onSelect}
              >
                <span className="report-chart-column-fill" />
                <span className="report-chart-column-amount">
                  {reading.amount}
                </span>
              </Button>
              <Button
                variant="link"
                className="report-chart-column report-chart-column-secondary"
                style={{
                  height: `${(Math.max(0, reading.comparison ?? 0) / scale) * 100}%`,
                }}
                aria-label={`${reading.label}, ${comparisonLabel}: ${reading.comparisonAmount ?? "—"}`}
                onClick={() => onSelect?.(`${reading.key}:comparison`)}
                disabled={!onSelect}
              >
                <span className="report-chart-column-fill" />
                <span className="report-chart-column-amount">
                  {reading.comparisonAmount}
                </span>
              </Button>
            </div>
            <span className="report-chart-column-label">{reading.label}</span>
          </div>
        ))}
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

function markGeometry(
  width: number,
  left = 0,
): CSSProperties & {
  "--report-mark-width": string;
  "--report-mark-left": string;
} {
  return {
    "--report-mark-width": `${width}%`,
    "--report-mark-left": `${left}%`,
  };
}
