// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { formatDateTime, formatDayRange } from "../../format/format";
import type { Locale, Translator } from "../../i18n";
import {
  type ReportingEvaluation,
  reportingPeriodIsEmpty,
} from "./reporting.model";

type Window = Readonly<{ start_at: string; end_at: string }>;

/**
 * The whole period and the cutoff its actuals run through, as two facts on
 * one line. A quarter to date then never reads as the whole quarter.
 * `through` is absent once a period has closed.
 */
export function PeriodCaption({
  period,
  through,
}: Readonly<{ period?: string; through?: string }>) {
  return (
    <>
      {period && <span>{period}</span>}
      {period && through && " · "}
      {through && <span>{through}</span>}
    </>
  );
}

// A window's end is exclusive, so the last day it holds is the one before.
function lastInstant(window: Window): string {
  return new Date(Date.parse(window.end_at) - 1).toISOString();
}

export function periodRange(
  window: Window,
  locale: Locale,
  zone: string,
): string {
  return formatDayRange(window.start_at, lastInstant(window), locale, zone);
}

/**
 * The caption of a reading's period. A running period names its whole span
 * and the cutoff. An answer stored before the span was kept names only the
 * cutoff, rather than passing the measured part off as the whole.
 */
export function periodCaption(
  evaluation: ReportingEvaluation | undefined,
  locale: Locale,
  t: Translator,
): ReactNode {
  if (!evaluation) return undefined;
  const { interval, horizon, evaluated_at, timezone } = evaluation.context;
  if (reportingPeriodIsEmpty(interval)) {
    return t("reporting.periodNotStarted", {
      at: formatDateTime(interval.start_at, locale, timezone),
    });
  }
  const running = interval.end_at === evaluated_at;
  const whole = horizon ?? (running ? undefined : interval);
  return (
    <PeriodCaption
      period={whole && periodRange(whole, locale, timezone)}
      through={
        running
          ? t("reporting.actualsThrough", {
              at: formatDateTime(interval.end_at, locale, timezone),
            })
          : undefined
      }
    />
  );
}
