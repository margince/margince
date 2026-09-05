// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId } from "react";
import type { components } from "../api/schema";
import { useDrawsImportRun } from "../app/import-onscreen";
import { Button, Radio, Skeleton } from "../design-system/atoms";
import { formatMoney, formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { type ImportWindow, isLiveRun, useBackfillRun } from "./backfill-run";
import { problemMessageOf } from "./common";
import { errorClassKey } from "./connector-status";
import "./onboarding-backread.css";

/**
 * The last act of onboarding: a mailbox is connected, so the flow asks how far
 * back to read it, starts the read, and shows what it found. Without this step
 * a fresh install knows nothing about relationships the user already has, which
 * is the empty-install problem the whole wizard exists to solve.
 *
 * Connecting and reading are two operations and the surface keeps them apart:
 * the grant is already given, this spends budget on history. So the scope is
 * previewed before the spend (ADR-0020) and the read is explicitly started.
 *
 * Three honesty rules run through every render here:
 *
 *  - `BackfillStatus` requires only `state`. Every count, the window and the
 *    denominator are optional, so each is absent-until-present: a tally with no
 *    count is omitted, never drawn as a 0 that looks like a finding.
 *  - progress is a fraction only when the server gave a denominator. Without
 *    `estimated_messages` the line is an open count and no percentage exists.
 *  - the run is a server-side job, so leaving is always available: the user can
 *    walk into the app with the read still going, and it keeps going.
 */

type BackfillStatus = components["schemas"]["BackfillStatus"];
type BackfillPreview = components["schemas"]["BackfillPreview"];
type BackfillCounts = NonNullable<BackfillStatus["counts"]>;
type Provider = components["schemas"]["CaptureConnection"]["provider"];

// The startable windows, in reach order. `none` is expressed by never starting
// a run at all — which is what the leave-without-reading control does, and
// which windows are startable is `ImportWindow`, beside the operations this
// step and the Settings card share.
const WINDOWS: readonly { value: ImportWindow; label: MessageKey }[] = [
  { value: "3m", label: "ob.backread.window3m" },
  { value: "6m", label: "ob.backread.window6m" },
  { value: "12m", label: "ob.backread.window12m" },
  { value: "24m", label: "ob.backread.window24m" },
  { value: "60m", label: "ob.backread.window60m" },
];

// The contract pins v1 estimates to USD minor units and leaves `currency`
// optional, so USD is the documented fallback rather than a guess. The symbol
// itself is never spelled here — Intl derives it from the code and the locale.
const FALLBACK_CURRENCY = "USD";

// Declaration order is render order. Every entry names a persisted count on the
// wire and the copy for it; `dedupe_candidates` is deliberately absent — it is
// review work, not a finding, and has no sentence in this step.
const TALLIES: readonly { key: keyof BackfillCounts; label: MessageKey }[] = [
  { key: "messages_scanned", label: "ob.backread.tallyMessages" },
  { key: "captured", label: "ob.backread.tallyCaptured" },
  { key: "skipped", label: "ob.backread.tallySkipped" },
  { key: "people_created", label: "ob.backread.tallyPeople" },
  { key: "organizations_created", label: "ob.backread.tallyCompanies" },
];

// The template-ready `{detail}` value for a mutation that may or may not
// have failed — `null` while it hasn't, the failure's reader-safe text once it
// has. Pulled out of the component itself so each of the three mutations'
// error handling reads as one call, not a ternary. It only derives text, so it
// stays safe to call from render as many times as render runs; keeping the raw
// failure readable is the client's mutation sink's job (app/queryclient.ts).
function safeDetail(
  isError: boolean,
  error: unknown,
  t: (key: MessageKey) => string,
): string | null {
  if (!isError) {
    return null;
  }
  return problemMessageOf(error, t, t("ob.backread.detailUnavailable"));
}

export type OnboardingBackreadProps = Readonly<{
  provider: Provider;
  /** The run row already embedded in the `GET /connectors` roster row — seeds
   *  the first render, so returning to a read in progress shows it immediately
   *  and never offers to start a second one. */
  initial?: BackfillStatus;
  /** Finish onboarding. `skipped` is the CONNECT step's flag, not the
   *  backread's: the mailbox is connected on every path through this surface,
   *  so declining the history read still finishes with `false`. */
  onFinish: (skipped: boolean) => void;
  /** Hold the start verb while a decision ABOUT this mailbox is still being
   *  written — the posture, today. A read that begins first imports under the
   *  answer the write was about to replace. */
  disabled?: boolean;
}>;

export function OnboardingBackread({
  provider,
  initial,
  disabled,
  onFinish,
}: OnboardingBackreadProps) {
  const t = useT();
  const importRun = useBackfillRun({ provider, initial });
  const { status, preview, start, cancel } = importRun;
  const selected = importRun.window;

  // This step draws the run in full, so the shell's capture chip stands down
  // for as long as it is on screen rather than gauging the same import twice.
  useDrawsImportRun(isLiveRun(status.data?.state));

  // `preview.data`/`preview.error` are the mutation's LAST result, which
  // survives past the render where `selected` changes to a window nobody has
  // previewed yet. `preview.variables` is the window that result actually
  // belongs to, so it gates both what scope is shown and whether Start may
  // fire: a stale estimate for the old window is withheld rather than shown
  // as though it answered the new pick, and Start waits for THIS selection's
  // preview to settle — successfully or not (an estimate that failed is still
  // a settled answer; see `BackreadScope`).
  const previewForSelection = preview.variables === selected;
  const previewSettled = previewForSelection && !preview.isPending;

  if (status.data === undefined) {
    // A failed status read must not trap anyone in the wizard: capture itself
    // is unaffected, so the honest answer is the missing view plus the exit.
    return status.isError ? (
      <section className="ob-backread">
        <p className="ob-backread-problem" role="alert">
          {t("backfill.statusUnavailable")}
        </p>
        <div className="ob-backread-acts">
          <Button variant="primary" onClick={() => onFinish(false)}>
            {t("ob.s4.enterCrm")}
          </Button>
        </div>
      </section>
    ) : (
      <div className="ob-backread">
        <Skeleton width="70%" />
        <Skeleton width="45%" />
      </div>
    );
  }

  if (importRun.isSetup) {
    return (
      <BackreadSetup
        selected={selected}
        onSelect={importRun.setWindow}
        // Gated on settled, not just on-selection: react-query already clears
        // `data` while a mutation is pending, but this does not depend on
        // that — the old window's estimate stays withheld on our own terms
        // even if a future call resolves pending differently.
        preview={previewSettled ? preview.data : undefined}
        previewProblem={
          previewForSelection
            ? safeDetail(preview.isError, preview.error, t)
            : null
        }
        previewReady={previewSettled}
        starting={start.isPending}
        held={disabled}
        startProblem={safeDetail(start.isError, start.error, t)}
        onStart={() => start.mutate(selected)}
        onFinish={onFinish}
      />
    );
  }

  const run = status.data;
  return (
    <BackreadRun
      run={run}
      cancelling={cancel.isPending}
      cancelProblem={safeDetail(cancel.isError, cancel.error, t)}
      onCancel={() => cancel.mutate()}
      onRestart={() => importRun.restart(run)}
      onFinish={onFinish}
    />
  );
}

// The window pick and the consent it carries. The read-only note sits ABOVE
// the start button on purpose: it is what makes the button safe to press, and a
// reassurance read after the click is a reassurance nobody read.
function BackreadSetup({
  selected,
  onSelect,
  preview,
  previewProblem,
  previewReady,
  starting,
  startProblem,
  held,
  onStart,
  onFinish,
}: Readonly<{
  selected: ImportWindow;
  onSelect: (pick: ImportWindow) => void;
  preview: BackfillPreview | undefined;
  previewProblem: string | null;
  /** A decision about this mailbox is still being written; the read must not
   *  begin ahead of it. Separate from `starting`, which also drives the verb's
   *  own copy — a button reading "starting…" when nothing started is a lie. */
  held?: boolean;
  /** True once THIS selection's own preview has settled (found or failed).
   *  Start waits for it so a read can never fire against a scope the reader
   *  has not actually seen. */
  previewReady: boolean;
  starting: boolean;
  startProblem: string | null;
  onStart: () => void;
  onFinish: (skipped: boolean) => void;
}>) {
  const t = useT();
  const group = useId();

  return (
    <section className="ob-backread">
      <h3 className="ob-backread-h t-h3">{t("ob.backread.heading")}</h3>
      <fieldset className="ob-backread-windows">
        <legend className="sr-only">{t("ob.backread.heading")}</legend>
        {WINDOWS.map((option) => (
          <Radio
            className="ob-backread-window"
            key={option.value}
            name={group}
            checked={selected === option.value}
            onChange={() => onSelect(option.value)}
            label={t(option.label)}
          />
        ))}
      </fieldset>
      <BackreadScope preview={preview} problem={previewProblem} />
      <p className="ob-backread-note t-caption">{t("ob.backread.note")}</p>
      <div className="ob-backread-acts">
        <Button
          variant="primary"
          disabled={starting || !previewReady || held}
          onClick={onStart}
        >
          {t("ob.backread.start")}
        </Button>
        <Button onClick={() => onFinish(false)}>{t("ob.backread.skip")}</Button>
      </div>
      {startProblem !== null && (
        <p className="ob-backread-problem" role="alert">
          {t("ob.backread.startFailed", { detail: startProblem })}
        </p>
      )}
    </section>
  );
}

// What the selected window would touch. An estimator that failed is stated and
// leaves the start available: not knowing the size of the mailbox is a reason
// to say so, never a reason to refuse the read.
function BackreadScope({
  preview,
  problem,
}: Readonly<{ preview: BackfillPreview | undefined; problem: string | null }>) {
  const t = useT();
  const { locale } = useLocale();
  // Absent when no model rate applied to this window — an unpriced estimate
  // says nothing about cost rather than saying zero.
  const cost =
    preview?.estimated_cost_minor === undefined
      ? null
      : formatMoney(
          preview.estimated_cost_minor,
          preview.currency ?? FALLBACK_CURRENCY,
          locale,
        );

  return (
    <div className="ob-backread-scope" aria-live="polite">
      {preview && (
        <p className="ob-backread-estimate">
          {t("ob.backread.estimate", {
            messages: formatNumber(preview.estimated_messages, locale),
          })}
        </p>
      )}
      {preview?.estimate_quality === "heuristic" && (
        <p className="ob-backread-qualifier t-caption">
          {t("ob.backread.estimateHeuristic")}
        </p>
      )}
      {cost !== null && (
        <p className="ob-backread-cost">
          {t("ob.backread.estimateCost", { cost })}
        </p>
      )}
      {problem !== null && (
        <p className="ob-backread-problem" role="alert">
          {t("ob.backread.estimateFailed", { detail: problem })}
        </p>
      )}
    </div>
  );
}

// Heading per state: a live read and a finished one each get one, while a
// failed or cancelled read leads with its sentence — "The backread stopped" is
// prose, and setting it as a heading would shout the failure.
function headingKey(state: BackfillStatus["state"]): MessageKey | null {
  if (isLiveRun(state)) return "ob.backread.running";
  return state === "done" ? "ob.backread.doneHeading" : null;
}

function BackreadRun({
  run,
  cancelling,
  cancelProblem,
  onCancel,
  onRestart,
  onFinish,
}: Readonly<{
  run: BackfillStatus;
  cancelling: boolean;
  cancelProblem: string | null;
  onCancel: () => void;
  /** Put the window pick back in front of the reader. Offered on every read
   *  this view draws that is not live: stopping one is a decision about that
   *  read, never about the mailbox. */
  onRestart: () => void;
  onFinish: (skipped: boolean) => void;
}>) {
  const t = useT();
  const heading = headingKey(run.state);
  const live = isLiveRun(run.state);

  return (
    <section className="ob-backread">
      {heading !== null && <h3 className="ob-backread-h t-h3">{t(heading)}</h3>}
      {live && <BackreadProgress run={run} />}
      <BackreadTallies counts={run.counts} />
      <BackreadOutcome run={run} />
      {cancelProblem !== null && (
        <p className="ob-backread-problem" role="alert">
          {t("ob.backread.cancelFailed", { detail: cancelProblem })}
        </p>
      )}
      <div className="ob-backread-acts">
        <Button variant="primary" onClick={() => onFinish(false)}>
          {live ? t("ob.backread.explore") : t("ob.s4.enterCrm")}
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

// The progress line, and a bar only when the server gave a denominator to
// divide by. The counts are stated in text and the bar merely draws them, so it
// is aria-hidden rather than a second, rounder version of the same number.
function BackreadProgress({ run }: Readonly<{ run: BackfillStatus }>) {
  const t = useT();
  const { locale } = useLocale();
  const scanned = run.counts?.messages_scanned;
  if (scanned === undefined) {
    // Nothing has been counted yet — the queued sentence carries the state, and
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

// Every count the run actually reported, and only those: a tally the wire left
// out is a measurement that does not exist yet, and printing 0 for it would
// report an emptiness the server never claimed.
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
      return (
        <p className="ob-backread-note t-caption">{t("ob.backread.queued")}</p>
      );
    case "running":
      return (
        <p className="ob-backread-note t-caption">
          {t("ob.backread.runningNote")}
        </p>
      );
    case "done":
      return (
        <p className="ob-backread-note t-caption">
          {t("ob.backread.doneNote")}
        </p>
      );
    case "error":
      return (
        <p className="ob-backread-problem" role="alert">
          {t("ob.backread.failed", {
            detail: t(errorClassKey(run.last_error_class)),
          })}
        </p>
      );
    case "cancelled": {
      // "Nothing was written" is only true of a cancel that landed before any
      // page finished. Once at least one row is `captured`, the read is
      // partial, not empty, and saying otherwise would be a false statement
      // about the reader's own data — the captured rows are already sitting
      // in the inbox, waiting on review, whether or not the run kept going.
      const captured = run.counts?.captured ?? 0;
      return (
        <p className="ob-backread-note t-caption">
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
