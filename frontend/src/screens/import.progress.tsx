// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Callout } from "../design-system/callout";
import { ProgressBar } from "../design-system/progressbar";
import { StatCard } from "../design-system/statcard";
import { StatStrip } from "../design-system/statstrip";
import { formatNumber } from "../format/format";
import { formatSpan } from "../format/now";
import { useLocale, useT } from "../i18n";
import type { Locale } from "../i18n/locale";
import { type ImportRun, isImportObject } from "./importtypes";

// Past this many seconds without a row, the panel says so. The server counts
// the seconds on its own clock, so a slow browser never reads as a stall.
const STALL_SECONDS = 30;

const SECONDS_PER_MINUTE = 60;

type Shown = Readonly<{
  run: ImportRun;
  minutes: number | null;
  lower: number | null;
}>;

// The minutes left, rounded up from the high bound. A lower figure shows only
// once two polls in a row agree on it; a higher one shows at once, so the
// estimate never flickers down and back up on one noisy reading.
function settleEta(before: Shown, run: ImportRun): Shown {
  const high = run.eta_seconds_high;
  if (run.eta_seconds_low === undefined || high === undefined) {
    return { run, minutes: null, lower: null };
  }
  const wanted = Math.max(1, Math.ceil(high / SECONDS_PER_MINUTE));
  if (before.minutes === null || wanted >= before.minutes) {
    return { run, minutes: wanted, lower: null };
  }
  if (before.lower === wanted) return { run, minutes: wanted, lower: null };
  return { run, minutes: before.minutes, lower: wanted };
}

function useSettledEta(run: ImportRun): number | null {
  const [shown, setShown] = useState<Shown>(() =>
    settleEta({ run, minutes: null, lower: null }, run),
  );
  if (shown.run === run) return shown.minutes;
  const next = settleEta(shown, run);
  setShown(next);
  return next.minutes;
}

function totalsOf(run: ImportRun) {
  const objects = run.progress?.objects ?? [];
  return {
    done: objects.reduce((sum, one) => sum + one.rows_done, 0),
    total: objects.reduce((sum, one) => sum + one.rows_total, 0),
  };
}

/** True when the run has said nothing but its status: an older API or worker. */
export function hasProgress(run: ImportRun): boolean {
  return run.phase !== undefined || run.progress !== undefined;
}

type Translate = ReturnType<typeof useT>;

// The reader's word for what the run is working on: a record type, the links
// pass, or a general word while the server names nothing.
function typeName(t: Translate, name: string | undefined): string {
  if (name === "links") return t("import.progress.links");
  return isImportObject(name)
    ? t(`import.object.${name}`)
    : t("import.progress.records");
}

// The one line of figures under the overall bar. A figure the server did not
// send is left out, so an older worker shows fewer words and none wrong.
function figuresLine(
  run: ImportRun,
  minutes: number | null,
  t: Translate,
  locale: Locale,
): string {
  const overall = totalsOf(run);
  const parts: (string | null)[] = [
    overall.total > 0
      ? t("import.progress.total", {
          done: formatNumber(overall.done, locale),
          total: formatNumber(overall.total, locale),
        })
      : null,
    run.rate_rows_per_second === undefined
      ? null
      : t("import.progress.rate", {
          rate: formatNumber(
            Math.max(1, Math.round(run.rate_rows_per_second)),
            locale,
          ),
        }),
    run.elapsed_seconds === undefined
      ? null
      : t("import.progress.elapsed", {
          span: formatSpan(run.elapsed_seconds * 1000, t, locale),
        }),
  ];
  if (minutes !== null) {
    const left = formatNumber(minutes, locale);
    parts.push(
      run.eta_scope === "object"
        ? t("import.progress.etaObject", {
            minutes: left,
            type: typeName(t, run.phase_object),
          })
        : t("import.progress.etaRun", { minutes: left }),
    );
  }
  return parts.filter((part) => part !== null).join(" · ");
}

function ObjectBars({ run }: Readonly<{ run: ImportRun }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <>
      {(run.progress?.objects ?? []).map((one) => {
        const label = typeName(t, one.object);
        const done = formatNumber(one.rows_done, locale);
        const total = formatNumber(one.rows_total, locale);
        return (
          <div key={one.object} className="import__part">
            <div className="import__partHead t-sub">
              <span>{label}</span>
              <span>
                {done} / {total}
              </span>
            </div>
            <ProgressBar
              fraction={one.rows_total > 0 ? one.rows_done / one.rows_total : 0}
              label={label}
              valueText={t("import.progress.rows", {
                type: label,
                done,
                total,
              })}
            />
          </div>
        );
      })}
    </>
  );
}

const COUNT_KEYS = [
  "created",
  "updated",
  "unchanged",
  "skipped",
  "problems",
] as const;

function Counts({
  counts,
}: Readonly<{ counts: NonNullable<ImportRun["progress"]>["counts"] }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <StatStrip label={t("import.progress.counts")}>
      {COUNT_KEYS.map((key) => (
        <StatCard
          key={key}
          label={t(`import.progress.count.${key}`)}
          value={formatNumber(counts[key], locale)}
        />
      ))}
    </StatStrip>
  );
}

/**
 * What a run that is moving has done so far: its phase, one bar overall and one
 * per object, the rate, the time spent and an estimate of the time left, the
 * counts, and a notice when it has made no progress for a while. Every figure
 * is the server's; a run that sends none reads as plain "Working…".
 */
export function ImportProgressPanel({
  run,
  waiting,
}: Readonly<{ run: ImportRun; waiting: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const minutes = useSettledEta(run);
  const queued = run.started_at === undefined && run.phase === undefined;
  const idle = run.idle_seconds ?? 0;
  const stalled = !queued && idle >= STALL_SECONDS;
  const phase = run.phase
    ? t(`import.phase.${run.phase}`, { type: typeName(t, run.phase_object) })
    : waiting;
  const serverSilent = (run.heartbeat_age_seconds ?? 0) >= STALL_SECONDS;
  const stalledTitle = serverSilent
    ? t("import.progress.stalled.server")
    : t("import.progress.stalled.working");
  // One region, whose text changes only with the phase and with a stall, so a
  // poll that moves the figures announces nothing.
  let announcement = queued ? t("import.progress.queued") : phase;
  if (stalled) announcement = stalledTitle;
  const overall = totalsOf(run);
  const figures = figuresLine(run, minutes, t, locale);
  return (
    <div className="import">
      <p className="sr-only" role="status">
        {announcement}
      </p>
      <p className="t-sub" aria-hidden>
        {queued ? t("import.progress.queued") : phase}
      </p>
      {overall.total > 0 ? (
        <ProgressBar
          fraction={overall.done / overall.total}
          label={t("import.progress.overall")}
          valueText={t("import.progress.total", {
            done: formatNumber(overall.done, locale),
            total: formatNumber(overall.total, locale),
          })}
        />
      ) : null}
      {figures === "" ? null : <p className="t-sub">{figures}</p>}
      <ObjectBars run={run} />
      {run.progress ? <Counts counts={run.progress.counts} /> : null}
      {stalled ? (
        <Callout tone="warning" kind="standing" title={stalledTitle}>
          {t("import.progress.stalled.body", {
            span: formatSpan(idle * 1000, t, locale),
          })}
        </Callout>
      ) : null}
    </div>
  );
}
