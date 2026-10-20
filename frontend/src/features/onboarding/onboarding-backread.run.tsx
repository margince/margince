import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { isLiveRun } from "../../lib/backfill-run";
import { errorClassKey } from "../../lib/connector-status";
import type { BackfillCounts, BackfillStatus } from "./onboarding-backread";

// Declaration order is render order. Every entry names a persisted count on the
// wire and the copy for it. The list is the whole of what the wire carries.
const TALLIES: readonly { key: keyof BackfillCounts; label: MessageKey }[] = [
  { key: "messages_scanned", label: "ob.backread.tallyMessages" },
  { key: "captured", label: "ob.backread.tallyCaptured" },
  { key: "skipped", label: "ob.backread.tallySkipped" },
  { key: "contacts_created", label: "ob.backread.tallyContacts" },
  { key: "companies_created", label: "ob.backread.tallyCompanies" },
];

// Heading per state: a live read and a finished one each get one. A failed or
// cancelled read leads with its sentence, because "The backread stopped" is
// prose, and as a heading it would shout the failure.
function headingKey(state: BackfillStatus["state"]): MessageKey | null {
  if (isLiveRun(state)) return "ob.backread.running";
  return state === "done" ? "ob.backread.doneHeading" : null;
}

export function BackreadRun({
  run,
  cancelling,
  cancelProblem,
  held,
  onCancel,
  onRestart,
  onDone,
}: Readonly<{
  run: BackfillStatus;
  cancelling: boolean;
  cancelProblem: string | null;
  /** A decision about this mailbox is still being written; the exit waits
   *  for it, for the reason the setup's does. */
  held?: boolean;
  onCancel: () => void;
  /** Put the window pick back in front of the reader. Offered on every read
   *  this view draws that is not live: stopping one is a decision about that
   *  read, never about the mailbox. */
  onRestart: () => void;
  onDone: () => void;
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
        <Button variant="primary" disabled={held} onClick={() => onDone()}>
          {live ? t("ob.backread.explore") : t("ob.conv.connect.dialogDone")}
        </Button>
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

// The progress line, and a bar only when the server gave a denominator. The
// counts are stated in text and the bar only draws them. So it is aria-hidden,
// not a second, rounder version of the same number.
function BackreadProgress({ run }: Readonly<{ run: BackfillStatus }>) {
  const t = useT();
  const { locale } = useLocale();
  const scanned = run.counts?.messages_scanned;
  if (scanned === undefined) {
    // Nothing has been counted yet. The queued sentence carries the state, and
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
        <div className="ob-backread-bar" aria-hidden="true">
          <span style={{ width: `${Math.min(1, scanned / total) * 100}%` }} />
        </div>
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
    <dl className="ob-backread-tallies">
      {present.map((tally) => (
        <div className="ob-backread-tally" key={tally.key}>
          <dt className="ob-backread-tally-label">{t(tally.label)}</dt>
          <dd className="ob-backread-tally-value">
            {formatNumber(tally.count, locale)}
          </dd>
        </div>
      ))}
    </dl>
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
      // "Nothing was written" is only true of a cancel that landed before any
      // page finished. Once at least one row is `captured`, the read is
      // partial, not empty. Saying otherwise would be false about the reader's
      // data. The captured rows already wait in the inbox for review, whether
      // or not the run kept going.
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
