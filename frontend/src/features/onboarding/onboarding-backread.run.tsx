// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Meter } from "../../design-system/readings";
import { StatCard } from "../../design-system/statcard";
import { StatStrip } from "../../design-system/statstrip";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { isLiveRun } from "../../lib/backfill-run";
import { errorClassKey } from "../../lib/connector-status";
import "./onboarding-backread.css";

// A started import as onboarding draws it: progress, what it found, and how it
// ended. Leaving is the step footer's. This view offers only the run's own
// verbs: stop a live run, or pick a window again over a stopped one.

type BackfillStatus = components["schemas"]["BackfillStatus"];
type BackfillCounts = NonNullable<BackfillStatus["counts"]>;

// Declaration order is render order. Every entry names a persisted count on the
// wire and the copy for it, and the list is the whole of what the wire carries.
const TALLIES: readonly { key: keyof BackfillCounts; label: MessageKey }[] = [
  { key: "messages_scanned", label: "ob.backread.tallyMessages" },
  { key: "captured", label: "ob.backread.tallyCaptured" },
  { key: "skipped", label: "ob.backread.tallySkipped" },
  { key: "contacts_created", label: "ob.backread.tallyContacts" },
  { key: "companies_created", label: "ob.backread.tallyCompanies" },
];

// Heading per state: a live read and a finished one each get one. A failed or
// cancelled read leads with its sentence: "The backread stopped" is prose, and
// a heading would shout the failure.
function headingKey(state: BackfillStatus["state"]): MessageKey | null {
  if (isLiveRun(state)) return "ob.backread.running";
  return state === "done" ? "ob.backread.doneHeading" : null;
}

export function BackreadRun({
  run,
  cancelling,
  cancelProblem,
  onCancel,
  onRestart,
}: Readonly<{
  run: BackfillStatus;
  cancelling: boolean;
  cancelProblem: string | null;
  onCancel: () => void;
  /** Put the window pick back in front of the reader. Offered on every read
   *  this view draws that is not live: stopping one is a decision about that
   *  read, never about the mailbox. */
  onRestart: () => void;
}>) {
  const t = useT();
  const heading = headingKey(run.state);
  const live = isLiveRun(run.state);

  return (
    <section className="ob-backread">
      {heading !== null && (
        <Heading size="medium" className="ob-backread-h t-h3">
          {t(heading)}
        </Heading>
      )}
      {live && <BackreadProgress run={run} />}
      <BackreadTallies counts={run.counts} />
      <BackreadOutcome run={run} />
      {cancelProblem !== null && (
        <ErrorLine>
          {t("ob.backread.cancelFailed", { detail: cancelProblem })}
        </ErrorLine>
      )}
      <div className="ob-backread-acts">
        {live ? (
          <Button disabled={cancelling} onClick={onCancel}>
            {t("ob.backread.cancel")}
          </Button>
        ) : (
          <Button onClick={onRestart}>{t("backfill.restart")}</Button>
        )}
      </div>
    </section>
  );
}

// The progress line, and a bar only when the server gave a denominator to
// divide by. The same Meter as the Settings import, so both bars read alike.
function BackreadProgress({ run }: Readonly<{ run: BackfillStatus }>) {
  const t = useT();
  const { locale } = useLocale();
  const scanned = run.counts?.messages_scanned;
  if (scanned === undefined) {
    // Nothing has been counted yet: the queued sentence carries the state, and
    // "0 messages so far" would claim a measurement nobody took.
    return null;
  }
  // A denominator of zero divides into nothing, so it is no denominator at all.
  const total = run.estimated_messages;
  const hasTotal = total !== undefined && total !== null && total > 0;

  return (
    <div className="ob-backread-progress" aria-live="polite">
      <p className="ob-backread-scanned">
        {hasTotal
          ? t("ob.backread.progress", {
              scanned: formatNumber(scanned, locale),
              total: formatNumber(total, locale),
            })
          : t("ob.backread.progressNoTotal", {
              scanned: formatNumber(scanned, locale),
            })}
      </p>
      {hasTotal && (
        <Meter
          value={Math.min(scanned, total)}
          max={total}
          label={t("backfill.progressLabel")}
          flat
        />
      )}
    </div>
  );
}

// Every count the run reported, and only those. A tally the wire left out is a
// measurement that does not exist yet. Printing 0 for it would report an
// emptiness the server never claimed.
function BackreadTallies({
  counts,
}: Readonly<{ counts: BackfillCounts | undefined }>) {
  const t = useT();
  const { locale } = useLocale();
  const present: {
    key: keyof BackfillCounts;
    label: MessageKey;
    count: number;
  }[] = [];
  for (const tally of TALLIES) {
    const count = counts?.[tally.key];
    if (count !== undefined) {
      present.push({ ...tally, count });
    }
  }
  if (present.length === 0) {
    return null;
  }

  return (
    <StatStrip density="compact">
      {present.map((tally) => (
        <StatCard
          key={tally.key}
          label={t(tally.label)}
          value={formatNumber(tally.count, locale)}
          narrow="row"
        />
      ))}
    </StatStrip>
  );
}

// What the run's state means for the reader, in one sentence. A failure names
// the provider-side class through the shared connector vocabulary rather than
// leaking the raw identifier, and says where to restart.
function BackreadOutcome({ run }: Readonly<{ run: BackfillStatus }>) {
  const t = useT();
  switch (run.state) {
    case "queued":
      return <p>{t("ob.backread.queued")}</p>;
    case "running":
      return <p>{t("ob.backread.runningNote")}</p>;
    case "done":
      return <p>{t("ob.backread.doneNote")}</p>;
    case "error":
      return (
        <ErrorLine>
          {t("ob.backread.failed", {
            detail: t(errorClassKey(run.last_error_class)),
          })}
        </ErrorLine>
      );
    case "cancelled": {
      // "Nothing was written" is only true of a cancel that landed before any page
      // finished. Once one row is `captured`, the read is partial, not empty. Those
      // rows already sit in the inbox waiting on review, whether or not the run
      // kept going.
      const captured = run.counts?.captured ?? 0;
      return (
        <p>
          {t(
            captured > 0
              ? "ob.backread.cancelledPartial"
              : "ob.backread.cancelled",
          )}
        </p>
      );
    }
    default:
      // `none` belongs to the setup view, which owns it before this renders. A
      // state a later server adds and this build cannot name says nothing,
      // rather than crashing the last step of onboarding.
      return null;
  }
}
