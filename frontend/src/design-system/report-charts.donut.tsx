// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type CSSProperties, useState } from "react";
import { formatPercent } from "../format/format";
import { useLocale } from "../i18n";
import { Button } from "./atoms";
import { type ReadingTone, readingColours } from "./readingtone";
import "./report-charts.donut.css";

/** One part of the whole the donut draws, its amount already formatted. */
export type DonutPart = Readonly<{
  key: string;
  label: string;
  value: number;
  amount: string;
  // A part that is a state takes that state's colour instead of a step.
  tone?: ReadingTone;
}>;

const SIZE = 160;
const STROKE = 22;
const RADIUS = (SIZE - STROKE) / 2;
const CIRCUMFERENCE = 2 * Math.PI * RADIUS;
// Track between two arcs, so neighbours of one hue still read as two.
const GAP = 3;
// The palest step still clears the track it sits on.
const PALEST = 32;

type ToneVars = CSSProperties & Record<`--${string}`, string>;

/**
 * Parts of one whole as a ring of arcs, from twelve o'clock and in the
 * caller's order, with the whole in its centre. Each part takes a step of one
 * hue, deepest first, since the parts are an order rather than categories.
 * The legend names every part with its amount and share. With `onSelect`
 * each legend row opens that part. Only parts above zero are drawn.
 */
export function DonutChart({
  label,
  parts,
  total,
  totalLabel,
  onSelect,
}: Readonly<{
  label: string;
  parts: readonly DonutPart[];
  // The whole, formatted, and the word under it.
  total: string;
  totalLabel: string;
  onSelect?: (key: string) => void;
}>) {
  const { locale } = useLocale();
  const [active, setActive] = useState<string | null>(null);
  const drawn = parts.filter((part) => part.value > 0);
  const sum = drawn.reduce((whole, part) => whole + part.value, 0);
  const arcs = arcsOf(drawn, sum);
  return (
    <figure
      className="report-donut"
      data-active={active ?? undefined}
      aria-label={label}
    >
      <div className="report-donut-dial">
        <svg viewBox={`0 0 ${SIZE} ${SIZE}`} aria-hidden="true">
          <circle className="report-donut-track" {...CIRCLE} />
          {arcs.map((arc) => (
            <circle
              key={arc.key}
              className="report-donut-arc"
              data-on={active === arc.key || undefined}
              {...CIRCLE}
              style={toneOf(arc.index, arcs.length, drawn[arc.index]?.tone)}
              strokeDasharray={`${arc.length} ${CIRCUMFERENCE}`}
              strokeDashoffset={-arc.start}
              onPointerEnter={() => setActive(arc.key)}
              onPointerLeave={() => setActive(null)}
            />
          ))}
        </svg>
        <span className="report-donut-centre">
          <span className="report-donut-total t-num">{total}</span>
          <span className="t-caption">{totalLabel}</span>
        </span>
      </div>
      <ul className="report-donut-legend">
        {drawn.map((part, index) => {
          const row = (
            <>
              <span
                className="report-donut-swatch"
                style={toneOf(index, drawn.length, part.tone)}
                aria-hidden="true"
              />
              <span className="report-donut-name">{part.label}</span>
              <span className="report-donut-amount t-num">{part.amount}</span>
              <span className="report-donut-share t-num t-caption">
                {formatPercent(part.value / sum, locale)}
              </span>
            </>
          );
          const hover = {
            onPointerEnter: () => setActive(part.key),
            onPointerLeave: () => setActive(null),
            onFocus: () => setActive(part.key),
            onBlur: () => setActive(null),
          };
          return (
            <li key={part.key} data-on={active === part.key || undefined}>
              {onSelect ? (
                <Button
                  variant="link"
                  className="report-donut-row"
                  aria-label={`${part.label}: ${part.amount}`}
                  onClick={() => onSelect(part.key)}
                  {...hover}
                >
                  {row}
                </Button>
              ) : (
                <span className="report-donut-row" {...hover}>
                  {row}
                </span>
              )}
            </li>
          );
        })}
      </ul>
    </figure>
  );
}

const CIRCLE = {
  cx: SIZE / 2,
  cy: SIZE / 2,
  r: RADIUS,
  fill: "none",
  strokeWidth: STROKE,
} as const;

type Arc = Readonly<{
  key: string;
  index: number;
  start: number;
  length: number;
}>;

// Each part's run along the circle, less the gap that follows it. A single
// part closes the ring with no gap.
function arcsOf(parts: readonly DonutPart[], sum: number): Arc[] {
  const gap = parts.length > 1 ? GAP : 0;
  let start = 0;
  return parts.map((part, index) => {
    const share = (part.value / sum) * CIRCUMFERENCE;
    const arc = {
      key: part.key,
      index,
      start,
      length: Math.max(share - gap, 1),
    };
    start += share;
    return arc;
  });
}

// Deepest for the first part, paling in even steps toward the last; a part
// with a tone wears it.
function toneOf(
  index: number,
  count: number,
  tone: ReadingTone | undefined,
): ToneVars {
  if (tone) return { "--report-donut-ink": readingColours(tone).mark };
  const step = count > 1 ? (100 - PALEST) / (count - 1) : 0;
  return { "--report-donut-share": `${Math.round(100 - index * step)}%` };
}
