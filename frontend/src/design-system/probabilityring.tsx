// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatPercent } from "../format/format";
import { useLocale, useT } from "../i18n";
import { fillDash } from "./ringarc";
import "./probabilityring.css";

const MINI = { size: 16, radius: 6, stroke: 2.5 } as const;

const CIRCLE = {
  cx: MINI.size / 2,
  cy: MINI.size / 2,
  r: MINI.radius,
  fill: "none",
  strokeWidth: MINI.stroke,
} as const;

/**
 * A deal's win probability inside a line of text: a small ring filled to the
 * percent, then the percent. The figure is the fact, so the ring stays out of
 * the accessible tree and the word for it is spoken, not drawn. `value` sets
 * the figure in the type and ink of the row it stands in, as a details row.
 */
export function ProbabilityRing({
  percent,
  value = false,
}: Readonly<{ percent: number; value?: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const fraction = Math.min(Math.max(percent, 0), 100) / 100;
  return (
    <span
      className={
        value ? "probability-ring probability-ring-value" : "probability-ring"
      }
    >
      <svg viewBox={`0 0 ${MINI.size} ${MINI.size}`} aria-hidden="true">
        <circle className="probability-ring-track" {...CIRCLE} />
        {fraction > 0 && (
          <circle
            className="probability-ring-fill"
            {...CIRCLE}
            strokeDasharray={fillDash(fraction, MINI.radius)}
          />
        )}
      </svg>
      <span className="sr-only">{t("deal.winProbability")}: </span>
      {formatPercent(fraction, locale)}
    </span>
  );
}
