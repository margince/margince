// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Building2, CheckCircle2, History, Mail, Users } from "lucide-react";
import type { components } from "../api/schema";
import { progressFraction } from "../app/capture-progress";
import { Badge, Button } from "../design-system/atoms";
import { CountUp } from "../design-system/countup";
import { formatDuration, formatNumber, formatPercent } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { isLiveRun } from "./backfill-run";
import "./backfill.css";

// What a capture run LOOKS like while it runs and once it has stopped: the
// three headline figures, the state it is in, and the share of the window it
// has read.
//
// Split from the setup panel beside it because the two answer different
// questions. That panel is the consent surface — a window, its scope, and the
// verb that spends; this one only ever reports what the server's own row
// already says, and every number in it is a persisted count.

type BackfillStatus = components["schemas"]["BackfillStatus"];

// A run whose updated_at hasn't moved in this long is honestly "stuck", not
// "in progress" — the contract's own doc comment on BackfillStatus.updated_at
// calls this out ("a killed worker leaves this honest"). Long enough that
// ordinary poll jitter or a slow provider batch never false-positives, short
// enough that a genuinely dead worker surfaces within a couple of polls of
// the threshold rather than staying "live" indefinitely.
const STALE_AFTER_MS = 3 * 60_000;

// A live run whose updated_at hasn't moved past STALE_AFTER_MS is honestly
// "stuck", not "in progress" — the contract's own doc comment on
// BackfillStatus.updated_at calls this out ("a killed worker leaves this
// honest"). A done/error/cancelled run's updated_at is its finish stamp, not
// a staleness signal, so this only applies to a live one.
function staleness(
  run: BackfillStatus,
  live: boolean,
): { stale: boolean; agoMs: number } {
  const agoMs = run.updated_at
    ? Math.max(0, Date.now() - new Date(run.updated_at).getTime())
    : 0;
  return {
    stale: live && run.updated_at != null && agoMs > STALE_AFTER_MS,
    agoMs,
  };
}

// The three headline figures of a capture run — captured mail and the two
// record kinds it grows. Each is a live persisted-row count. While the run is
// still reading, the figure is a `CountUp`: a number still being earned climbs
// to where the poll put it instead of jumping there. Once the run has settled
// it is the plain number, because a figure the server has finished with has
// nothing left to count towards.
const CAPTURE_STATS: {
  key: "captured" | "contacts_created" | "companies_created";
  label: MessageKey;
  icon: typeof Mail;
}[] = [
  { key: "captured", label: "backfill.statEmails", icon: Mail },
  { key: "contacts_created", label: "backfill.statContacts", icon: Users },
  {
    key: "companies_created",
    label: "backfill.statCompanies",
    icon: Building2,
  },
];

function CaptureStat({
  value,
  label,
  icon: Icon,
  locale,
  counting,
}: {
  value: number;
  label: string;
  icon: typeof Mail;
  locale: Locale;
  counting: boolean;
}) {
  return (
    <div className="capture-stat">
      <span className="capture-stat-glyph" aria-hidden>
        <Icon />
      </span>
      <b className="capture-stat-value t-display">
        {counting ? (
          <CountUp value={value} locale={locale} />
        ) : (
          formatNumber(value, locale)
        )}
      </b>
      <span className="capture-stat-label">{label}</span>
    </div>
  );
}

export function RunView({
  run,
  cancelling,
  cancelError,
  onCancel,
  onRestart,
}: {
  run: BackfillStatus;
  cancelling: boolean;
  cancelError: string | null;
  onCancel: () => void;
  // Put the window picker back in front of the reader. Offered on every run
  // that has stopped, which is every state this view draws that is not live:
  // stopping an import is a decision about this run, never about the mailbox.
  onRestart: () => void;
}) {
  const t = useT();
  const { locale } = useLocale();
  const counts = run.counts;
  const scanned = counts?.messages_scanned ?? 0;
  const live = isLiveRun(run.state);
  const done = run.state === "done";
  const { stale, agoMs } = staleness(run, live);
  // The card wears the AI family only while the machine is actually reading:
  // indigo is a claim about who is doing the work, so a queued run that has
  // not started and a stalled one that has stopped both stay on plain ground.
  const reading = run.state === "running" && !stale;
  // A percentage needs a denominator that is still true, and whether this one
  // is depends on what kind of number the provider gave: Gmail counts by
  // paging under a cap and answers a FLOOR on a large mailbox, Graph answers
  // an exact count. progressFraction is the one rule — shared with the
  // chrome's own orb, because a bar that differed between the two would be two
  // answers to one question on one screen. A run that is not moving forward
  // gets no bar at all, which is this screen's own addition to it: a stalled
  // import must not draw something that implies progress.
  const fraction =
    live && !stale
      ? progressFraction({
          scanned,
          estimated: run.estimated_messages ?? null,
          estimatedIsFloor: run.estimate_is_floor === true,
        })
      : null;
  const heroClass = ["capture-hero", done && "done", reading && "reading"]
    .filter(Boolean)
    .join(" ");

  return (
    <div className={heroClass}>
      <RunHead state={run.state} reading={reading} />
      <div className="capture-stats">
        {CAPTURE_STATS.map((stat) => (
          <CaptureStat
            key={stat.key}
            value={counts?.[stat.key] ?? 0}
            label={t(stat.label)}
            icon={stat.icon}
            locale={locale}
            counting={reading}
          />
        ))}
      </div>
      <RunProgress
        scanned={scanned}
        fraction={fraction}
        staleForMs={stale ? agoMs : null}
      />
      {run.state === "error" && (
        <p className="t-caption backfill-error">
          {t("backfill.errorNote")}
          {run.last_error_class ? ` (${run.last_error_class})` : ""}
        </p>
      )}
      <div className="backfill-foot">
        {live ? (
          <Button small disabled={cancelling} onClick={onCancel}>
            {t("backfill.cancel")}
          </Button>
        ) : (
          <Button small onClick={onRestart}>
            {t("backfill.restart")}
          </Button>
        )}
      </div>
      {live && cancelError && (
        <p className="t-caption backfill-error">{cancelError}</p>
      )}
      {run.state === "cancelled" && (
        <p className="t-caption">{t("backfill.cancelledNote")}</p>
      )}
    </div>
  );
}

// The state's glyph on its disc, the title, and — only while the machine is
// reading — the pill that says so in words, because the indigo the card wears
// then is a claim about who is doing the work and colour is never the only
// signal.
function RunHead({
  state,
  reading,
}: {
  state: BackfillStatus["state"];
  reading: boolean;
}) {
  const t = useT();
  return (
    <div className="capture-head" aria-live="polite">
      <span className="capture-mark" aria-hidden>
        {state === "done" ? (
          <CheckCircle2 />
        ) : (
          <History className={reading ? "spin-slow" : ""} />
        )}
      </span>
      <h3 className="backfill-h">{t(stateTitle(state))}</h3>
      {reading && (
        <span className="capture-head-tag">
          <Badge tone="ai">{t("backfill.readingBadge")}</Badge>
        </span>
      )}
    </div>
  );
}

// Either the bar or the staleness note over the scanned line, never both: a run
// that is not moving forward does not get to keep the bar that implies
// otherwise. The percentage rides the line only when the bar is drawn — the two
// state one number twice, in a shape and in words.
function RunProgress({
  scanned,
  fraction,
  staleForMs,
}: {
  scanned: number;
  fraction: number | null;
  // How long a live run has gone without moving, or null while it moves.
  staleForMs: number | null;
}) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <>
      {staleForMs !== null && (
        <p className="t-caption backfill-stale">
          {t("backfill.staleUpdated", {
            duration: formatDuration(staleForMs, locale),
          })}
        </p>
      )}
      {fraction !== null && <RunBar fraction={fraction} />}
      <p className="t-caption capture-scanned" aria-live="polite">
        <span>
          {t("backfill.countScanned")} {formatNumber(scanned, locale)}
        </span>
        {fraction !== null && (
          <span className="capture-pct">{formatPercent(fraction, locale)}</span>
        )}
      </p>
    </>
  );
}

// The bar draws the fraction the line under it states in words, and exposes
// the same number to assistive tech through the progressbar role rather than
// through a native `<progress>`, whose track and fill are the browser's colours
// and the one thing on this card no token could reach.
function RunBar({ fraction }: { fraction: number }) {
  const t = useT();
  const percent = Math.round(fraction * 100);
  return (
    <div
      className="capture-bar"
      role="progressbar"
      aria-label={t("backfill.progressLabel")}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={percent}
    >
      <span
        className="capture-bar-fill"
        style={{ inlineSize: `${percent}%` }}
      />
    </div>
  );
}

function stateTitle(state: BackfillStatus["state"]): MessageKey {
  switch (state) {
    case "queued":
      return "backfill.queuedTitle";
    case "running":
      return "backfill.runningTitle";
    case "error":
      return "backfill.errorTitle";
    case "cancelled":
      return "backfill.cancelledTitle";
    default:
      return "backfill.doneTitle";
  }
}
