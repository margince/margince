// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody } from "../../design-system/panel";
import { ProgressBar } from "../../design-system/progressbar";
import { StatCard } from "../../design-system/statcard";
import { StatStrip } from "../../design-system/statstrip";
import { StatusBadge } from "../../design-system/statusbadge";
import { formatNumber, formatPercent } from "../../format/format";
import { formatSpan } from "../../format/now";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { isLiveRun } from "../../lib/backfill-run";
import { progressFraction } from "../../lib/capture-progress";
import "../../components/backfill.css";

// What a capture run looks like while it runs and once it has stopped. It shows
// the three headline figures, the state it is in, and the share of the window
// read.
//
// Split from the setup panel beside it, which is the consent surface. This one
// only reports what the server's own row says, and every number in it is a
// persisted count.

type BackfillStatus = components["schemas"]["BackfillStatus"];

// A run whose updated_at hasn't moved in this long is "stuck", not "in
// progress". Long enough that poll jitter or a slow provider batch never
// false-positives, short enough that a dead worker surfaces within a couple of
// polls.
const STALE_AFTER_MS = 3 * 60_000;

// A live run whose updated_at hasn't moved past STALE_AFTER_MS is "stuck", not
// "in progress". A done/error/cancelled run's updated_at is its finish stamp,
// not a staleness signal, so this only applies to a live one.
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

// The three headline figures of a capture run: captured mail and the two
// record kinds it grows, each a persisted-row count.
const CAPTURE_STATS: {
  key: "captured" | "contacts_created" | "companies_created";
  label: MessageKey;
}[] = [
  { key: "captured", label: "backfill.statEmails" },
  { key: "contacts_created", label: "backfill.statContacts" },
  { key: "companies_created", label: "backfill.statCompanies" },
];

/** What a run row says about itself, read the one way every surface that
 *  draws an import reads it: this hero, and the Past email service on a
 *  Connections account. */
export function runReading(run: BackfillStatus) {
  const counts = run.counts;
  const scanned = counts?.messages_scanned ?? 0;
  const live = isLiveRun(run.state);
  const { stale, agoMs } = staleness(run, live);
  // A percentage needs a denominator that is still true. That depends on the
  // provider: Gmail answers a floor on a large mailbox, Graph an exact count.
  // progressFraction is the one rule, shared with the chrome's own orb. A run
  // that is not moving forward gets no bar, since it would imply progress.
  const fraction =
    live && !stale
      ? progressFraction({
          scanned,
          estimated: run.estimated_messages ?? null,
          estimatedIsFloor: run.estimate_is_floor === true,
        })
      : null;
  return {
    scanned,
    // Messages the run walked past because they could not be captured. Said
    // rather than folded into the other figures, so a gap has a number.
    failed: counts?.failed ?? 0,
    live,
    stale,
    agoMs,
    // A queued run that has not started and a stalled one are not reading.
    reading: run.state === "running" && !stale,
    fraction,
  };
}

/** A run that stopped on an error and kept its place, so it can continue. */
export function canResume(run: BackfillStatus): boolean {
  return run.state === "error" && run.resumable === true;
}

export function RunView({
  run,
  cancelling,
  cancelError,
  onCancel,
  onRestart,
  onResume,
  resuming = false,
  resumeError = null,
}: {
  run: BackfillStatus;
  cancelling: boolean;
  cancelError: string | null;
  onCancel: () => void;
  // Put the window picker back in front of the reader. Offered on every run
  // that has stopped, which is every state this view draws that is not live.
  // Stopping an import is a decision about this run, never about the mailbox.
  onRestart: () => void;
  // Continue a run that stopped on an error from where it stopped. Offered only
  // when the server says the run can be continued (`resumable`).
  onResume?: () => void;
  resuming?: boolean;
  resumeError?: string | null;
}) {
  const t = useT();
  const { locale } = useLocale();
  const counts = run.counts;
  const { scanned, failed, live, stale, agoMs, reading, fraction } =
    runReading(run);
  // A run that stopped on an error and kept its place is offered as one to
  // continue, with starting over as the second choice.
  const resumable = canResume(run) && onResume !== undefined;

  return (
    <Panel
      title={t(stateTitle(run.state))}
      titleLevel={3}
      titleAction={
        reading ? (
          <StatusBadge kind="neutral" live label={t("backfill.readingBadge")} />
        ) : undefined
      }
      actions={
        <RunFoot
          live={live}
          resumable={resumable}
          cancelling={cancelling}
          resuming={resuming}
          onCancel={onCancel}
          onResume={onResume}
          onRestart={onRestart}
        />
      }
    >
      <PanelBody className="form-stack">
        <StatStrip density="compact">
          {CAPTURE_STATS.map((stat) => (
            <StatCard
              key={stat.key}
              label={t(stat.label)}
              value={formatNumber(counts?.[stat.key] ?? 0, locale)}
              narrow="row"
            />
          ))}
        </StatStrip>
        <RunProgress
          scanned={scanned}
          fraction={fraction}
          staleForMs={stale ? agoMs : null}
        />
        <RunNotes
          run={run}
          scanned={scanned}
          failed={failed}
          resumable={resumable}
        />
        {resumable && resumeError && <ErrorLine>{resumeError}</ErrorLine>}
        {live && cancelError && <ErrorLine>{cancelError}</ErrorLine>}
        {run.state === "cancelled" && <p>{t("backfill.cancelledNote")}</p>}
      </PanelBody>
    </Panel>
  );
}

// What a stopped run has to say beyond its figures. That is the error, the
// messages it walked past, and where a run that can be continued stopped.
function RunNotes({
  run,
  scanned,
  failed,
  resumable,
}: {
  run: BackfillStatus;
  scanned: number;
  failed: number;
  resumable: boolean;
}) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <>
      {run.state === "error" && (
        <ErrorLine>
          {t(resumable ? "backfill.errorResumeNote" : "backfill.errorNote")}
          {run.last_error_class ? ` (${run.last_error_class})` : ""}
        </ErrorLine>
      )}
      {failed > 0 && (
        <p className="t-caption">
          {plural("backfill.failedNote", failed, {
            count: formatNumber(failed, locale),
          })}
        </p>
      )}
      {resumable && (
        <p>
          {plural("backfill.resumeNote", scanned, {
            count: formatNumber(scanned, locale),
          })}
        </p>
      )}
    </>
  );
}

// The run's verbs: stop a live one; continue or start over a stopped one that
// can be continued; start a new one otherwise.
function RunFoot({
  live,
  resumable,
  cancelling,
  resuming,
  onCancel,
  onResume,
  onRestart,
}: {
  live: boolean;
  resumable: boolean;
  cancelling: boolean;
  resuming: boolean;
  onCancel: () => void;
  onResume?: () => void;
  onRestart: () => void;
}) {
  const t = useT();
  return (
    <>
      {live && (
        <Button disabled={cancelling} onClick={onCancel}>
          {t("backfill.cancel")}
        </Button>
      )}
      {resumable && (
        <Button
          variant="primary"
          pending={resuming}
          busyLabel={t("backfill.starting")}
          onClick={onResume}
        >
          {t("backfill.resumeCta")}
        </Button>
      )}
      {!live && (
        <Button disabled={resuming} onClick={onRestart}>
          {t(resumable ? "backfill.startOverCta" : "backfill.restart")}
        </Button>
      )}
    </>
  );
}

// Either the bar or the staleness note over the scanned line, never both. A run
// that is not moving forward does not keep the bar that implies otherwise. The
// percentage rides the line only when the bar is drawn, because the two state
// one number twice, in a shape and in words.
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
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <>
      {staleForMs !== null && (
        <p className="t-caption">
          {t("backfill.staleUpdated", {
            duration: formatSpan(staleForMs, t, locale),
          })}
        </p>
      )}
      {fraction !== null && (
        <ProgressBar
          fraction={fraction}
          label={t("backfill.progressLabel")}
          valueText={formatPercent(fraction, locale)}
        />
      )}
      <p className="t-caption capture-scanned" aria-live="polite">
        <span>
          {plural("backfill.countScanned", scanned, {
            count: formatNumber(scanned, locale),
          })}
        </span>
        {fraction !== null && <span>{formatPercent(fraction, locale)}</span>}
      </p>
    </>
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
