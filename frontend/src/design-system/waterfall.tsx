// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button, TableScroll } from "./atoms";
import "./waterfall.css";

export type WaterfallAnchor = Readonly<{
  label: string;
  value: number;
  amount: string;
}>;

export type WaterfallStep = WaterfallAnchor & Readonly<{ key: string }>;

type PositionedBar = WaterfallAnchor &
  Readonly<{
    key: string;
    start: number;
    end: number;
    anchor: boolean;
  }>;

function positionBars(
  opening: WaterfallAnchor,
  closing: WaterfallAnchor,
  steps: readonly WaterfallStep[],
): PositionedBar[] {
  let running = opening.value;
  const movement = steps.map((step) => {
    const start = running;
    running += step.value;
    return { ...step, start, end: running, anchor: false };
  });
  return [
    { ...opening, key: "opening", start: 0, end: opening.value, anchor: true },
    ...movement,
    { ...closing, key: "closing", start: 0, end: closing.value, anchor: true },
  ];
}

export function Waterfall({
  opening,
  closing,
  steps,
  label,
  reconciliationWarning,
  onSelect,
}: Readonly<{
  opening: WaterfallAnchor;
  closing: WaterfallAnchor;
  steps: readonly WaterfallStep[];
  label: string;
  reconciliationWarning: string;
  onSelect?: (key: string) => void;
}>) {
  const bars = positionBars(opening, closing, steps);
  const reconciles =
    opening.value + steps.reduce((sum, step) => sum + step.value, 0) ===
    closing.value;
  const low = Math.min(0, ...bars.flatMap((bar) => [bar.start, bar.end]));
  const high = Math.max(0, ...bars.flatMap((bar) => [bar.start, bar.end]));
  const span = Math.max(high - low, 1);
  const y = (value: number) => ((value - low) / span) * 100;

  return (
    <div className="report-chart">
      <TableScroll className="waterfall-scroll" label={label}>
        <ol
          className="waterfall-bars"
          aria-hidden={onSelect ? undefined : true}
        >
          {bars.map((bar, index) => (
            <li
              key={bar.key}
              className={`waterfall-bar waterfall-${bar.anchor ? "anchor" : bar.value < 0 ? "down" : "up"}`}
            >
              <div className="waterfall-plot">
                {onSelect && (
                  <Button
                    variant="link"
                    className="waterfall-mark-action"
                    aria-label={`${bar.label}: ${bar.amount}`}
                    onClick={() => onSelect(bar.key)}
                  />
                )}
                <span
                  className="waterfall-zero"
                  style={{ bottom: `${y(0)}%` }}
                />
                <span
                  className="waterfall-fill"
                  style={{
                    bottom: `${y(Math.min(bar.start, bar.end))}%`,
                    height: `${(Math.abs(bar.end - bar.start) / span) * 100}%`,
                  }}
                />
                {index < bars.length - 1 && (
                  <span
                    className="waterfall-connector"
                    style={{ bottom: `${y(bar.end)}%` }}
                  />
                )}
              </div>
              <span className="waterfall-label">
                <span>{bar.label}</span>
                <strong>{bar.amount}</strong>
              </span>
            </li>
          ))}
        </ol>
      </TableScroll>
      {!reconciles && (
        <p className="waterfall-warning" role="status">
          {reconciliationWarning}
        </p>
      )}
      <table className="sr-only">
        <caption>{label}</caption>
        <tbody>
          {bars.map((bar) => (
            <tr key={bar.key}>
              <th scope="row">{bar.label}</th>
              <td>{bar.amount}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
