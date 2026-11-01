// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { CSSProperties } from "react";
import { Button } from "./atoms";
import "./report-charts.css";

import type { ChartProps, GroupedBarsProps } from "./report-charts.model";
import { readingScale } from "./report-charts.model";
import { ReadingTable } from "./report-charts.table";

export { DonutChart, type DonutPart } from "./report-charts.donut";
export { CumulativeChart } from "./report-charts.line";
export type { ChartReading } from "./report-charts.model";

export function BulletChart({
  readings,
  label,
  dataLabel,
  valueLabel,
  targetLabel,
  onSelect,
}: ChartProps & Readonly<{ targetLabel: string }>) {
  const scale = readingScale(readings) * 1.08;
  const hasTargets = readings.some((reading) => reading.target != null);
  return (
    <figure className="report-chart">
      <figcaption className="sr-only">{label}</figcaption>
      <div className="report-chart-legend">
        <span className="report-chart-key">{valueLabel}</span>
        {hasTargets && (
          <span className="report-chart-key report-chart-key-target">
            {targetLabel}
          </span>
        )}
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
              aria-label={
                reading.target != null
                  ? `${reading.label}: ${reading.amount}; ${targetLabel}: ${reading.targetAmount ?? "—"}`
                  : `${reading.label}: ${reading.amount}`
              }
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
                {reading.target != null && (
                  <small>{reading.targetAmount}</small>
                )}
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
        secondaryLabel={hasTargets ? targetLabel : undefined}
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
  size = "default",
}: GroupedBarsProps) {
  const scale = readingScale(readings) * 1.15;
  return (
    <figure className="report-chart" data-size={size}>
      <figcaption className="sr-only">{label}</figcaption>
      <div className="report-chart-legend">
        <span className="report-chart-key report-chart-key-bar">
          {valueLabel}
        </span>
        {comparisonLabel && (
          <span className="report-chart-key report-chart-key-bar report-chart-key-bar-secondary">
            {comparisonLabel}
          </span>
        )}
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
              {comparisonLabel && (
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
              )}
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
