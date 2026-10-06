// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { ErrorLine } from "../design-system/errorline";
import { SegmentBar, type SegmentBarParts } from "../design-system/readings";
import { formatNumber, formatPercent } from "../format/format";
import { type Locale, useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { formatSeconds } from "./ai-call-figures";
import "./ai-settings.css";

// What one task's calls did over a window: how many got an answer, from which
// step of the route, and why the walk moved past each step. Every figure is
// the server's flow read; this file only lays it out.

type Flow = components["schemas"]["AiTaskFlow"];
type Step = components["schemas"]["AiFlowStep"];

/** Why a step gave a call up, in words; an unknown reason reads as itself. */
const GAVE_UP: Readonly<Record<string, MessageKey>> = {
  timeout: "aiOutcome.gaveUp.timeout",
  provider_error: "aiOutcome.gaveUp.failed",
  provider_throttled: "aiOutcome.gaveUp.throttled",
  provider_quota: "aiOutcome.gaveUp.quota",
  provider_refused: "aiOutcome.gaveUp.refused",
  decision_error: "aiOutcome.gaveUp.failed",
  decision_below_floor: "aiOutcome.gaveUp.unsure",
  decision_off_enum: "aiOutcome.gaveUp.offEnum",
  schema_invalid: "aiOutcome.gaveUp.invalid",
};

function share(part: number, total: number, locale: Locale): string {
  if (!total) return formatPercent(0, locale);
  const fraction = part / total;
  return fraction > 0 && fraction < 0.01
    ? `<${formatPercent(0.01, locale)}`
    : formatPercent(fraction, locale);
}

export function TaskOutcome({ flow }: Readonly<{ flow: Flow }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const total = flow.total;
  if (!total) return <p className="t-caption">{t("aiFigures.empty")}</p>;
  const [first, ...rest] = flow.steps;
  const firstTry = first?.answered ?? 0;
  const byFallback = rest.reduce((sum, step) => sum + step.answered, 0);
  const lost = flow.unanswered;
  const number = (n: number) => formatNumber(n, locale);
  const parts: SegmentBarParts = [
    {
      key: "first",
      label: t("aiOutcome.firstTry"),
      value: firstTry,
      amount: `${number(firstTry)} (${share(firstTry, total, locale)})`,
    },
    {
      key: "fallback",
      label: t("aiOutcome.fallback"),
      value: byFallback,
      amount: `${number(byFallback)} (${share(byFallback, total, locale)})`,
    },
    {
      key: "lost",
      label: t("aiOutcome.noAnswer"),
      value: lost,
      amount: `${number(lost)} (${share(lost, total, locale)})`,
    },
  ];
  return (
    <div className="form-stack">
      <p>
        {lost
          ? t("aiOutcome.headline.lost", {
              share: share(total - lost, total, locale),
              total: number(total),
              lost: number(lost),
            })
          : plural("aiOutcome.headline.all", total, { total: number(total) })}
      </p>
      <SegmentBar label={t("aiOutcome.legend")} parts={parts} />
      <ol className="ai-outcome-steps">
        {flow.steps.map((step, i) => (
          <StepCard
            key={`${step.decision}-${step.tier}`}
            step={step}
            index={i}
            last={i === flow.steps.length - 1}
          />
        ))}
      </ol>
      {lost ? (
        <ErrorLine standing>
          {plural("aiOutcome.lostNote", lost, { lost: number(lost) })}
        </ErrorLine>
      ) : null}
    </div>
  );
}

function StepCard({
  step,
  index,
  last,
}: Readonly<{ step: Step; index: number; last: boolean }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const role = step.decision
    ? t("aiOutcome.role.decision")
    : index === 0
      ? t("aiOutcome.role.tier")
      : t("aiOutcome.role.fallback");
  const reasons = Object.entries(step.gave_up);
  return (
    <li className="ai-outcome-step">
      <span className="t-caption">
        {t("aiOutcome.step", { n: formatNumber(index + 1, locale), role })}
      </span>
      <code className="t-caption">{step.model || step.tier}</code>
      {step.attempts ? (
        <>
          <span className="ai-outcome-big">
            {t("aiOutcome.answered", {
              answered: formatNumber(step.answered, locale),
              attempts: formatNumber(step.attempts, locale),
            })}
          </span>
          <span className="t-caption">
            {t("aiOutcome.usually", {
              latency: formatSeconds(step.p50_ms, locale),
            })}
          </span>
          {reasons.map(([reason, count]) => (
            <span
              key={reason}
              // ds:ignore a give-up count in the danger ink, not a message
              className={`t-caption ${last ? "ai-figures-bad" : "ai-outcome-passed"}`}
            >
              {plural(
                last ? "aiOutcome.gaveUp.last" : "aiOutcome.gaveUp.on",
                count,
                {
                  count: formatNumber(count, locale),
                  reason: GAVE_UP[reason] ? t(GAVE_UP[reason]) : reason,
                },
              )}
            </span>
          ))}
        </>
      ) : (
        <span className="t-caption">{t("aiOutcome.notNeeded")}</span>
      )}
    </li>
  );
}

/**
 * How long calls take against the timeout that stops them: the median and the
 * 95th percentile on one track with the timeout's line. A timeout ten times
 * past the slowest call is not drawn — the track would be all air — and the
 * caption says so instead.
 */
export function LatencyAgainstTimeout({
  p50Ms,
  p95Ms,
  timeoutMs,
  decision,
}: Readonly<{
  p50Ms: number;
  p95Ms: number;
  timeoutMs: number;
  decision: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const far = timeoutMs > p95Ms * 10;
  const scale = far
    ? Math.max(p95Ms, 1) * 1.5
    : Math.max(timeoutMs, p95Ms, 1) * 1.15;
  const at = (ms: number) => `${Math.min(100, (ms / scale) * 100).toFixed(1)}%`;
  const over = p95Ms >= timeoutMs;
  const near = !over && p95Ms >= timeoutMs * 0.8;
  const name = decision
    ? t("aiOutcome.decisionTimeout")
    : t("aiOutcome.timeout");
  const seconds = formatNumber(Math.round(timeoutMs / 1000), locale);
  return (
    <div className="ai-latency">
      <span className="ai-latency-title">{t("aiOutcome.latencyTitle")}</span>
      <div className="ai-latency-track" aria-hidden="true">
        <span className="ai-latency-mark" style={{ left: at(p50Ms) }} />
        <span
          className="ai-latency-mark ai-latency-p95"
          style={{ left: at(p95Ms) }}
        />
        {far ? null : (
          <span className="ai-latency-limit" style={{ left: at(timeoutMs) }} />
        )}
      </div>
      <p className="t-caption ai-latency-caption">
        <span>
          {t("aiOutcome.latencyMarks", {
            p50: formatSeconds(p50Ms, locale),
            p95: formatSeconds(p95Ms, locale),
          })}
        </span>
        <span
          // ds:ignore a latency reading in the danger ink, not a message
          className={
            over ? "ai-figures-bad" : near ? "ai-latency-near" : undefined
          }
        >
          {far
            ? t("aiOutcome.limitFar", { name, seconds })
            : over
              ? t("aiOutcome.limitOver", { name, seconds })
              : near
                ? t("aiOutcome.limitNear", { name, seconds })
                : t("aiOutcome.limit", { name, seconds })}
        </span>
      </p>
    </div>
  );
}
