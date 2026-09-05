import { Building2, CheckCircle2, History, Mail, Users } from "lucide-react";
import { useState } from "react";
import type { components } from "../api/schema";
import { useDrawsImportRun } from "../app/import-onscreen";
import { Badge, Button } from "../design-system/atoms";
import { ChoiceList } from "../design-system/choicelist";
import { CountUp } from "../design-system/countup";
import {
  formatDuration,
  formatMoney,
  formatNumber,
  formatPercent,
} from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { type ImportWindow, isLiveRun, useBackfillRun } from "./backfill-run";
import { ProblemError, problemCode, problemMessageOf } from "./common";
import "./backfill.css";

// The bounded connect-time backfill (ADR-0063): pick a window, see the scope
// BEFORE anything spends (ADR-0020 preview-before-spend — the estimate card
// is the consent surface), then watch real progress. Every number rendered
// here is a persisted-row count from the single-row status read; nothing is
// fabricated client-side (CAP-AC-OPEN-1). The scope preview auto-loads so the
// first thing a newly-connected user sees is honest scope, not a blank form —
// but the spend still waits for the explicit "Start the import" consent.
//
// Its one caller today is the Settings connected-inboxes card, which already
// holds the run row via the embedded `CaptureConnection.backfill` — seeding
// from it renders a live run with no extra request. Without a seed (the shape
// the onboarding coldstart used) it simply fetches on mount.
//
// It imports its own sheet. Every class it names — .backfill-setup,
// .backfill-h, .capture-hero, .capture-stat — used to be declared in
// onboarding.css, which this file has never imported and whose screen no longer
// mounts this panel at all, so the only caller was rendering it entirely
// unstyled.

type BackfillStatus = components["schemas"]["BackfillStatus"];
type Provider = components["schemas"]["CaptureConnection"]["provider"];

// The CAP-PARAM-4 set, in reach order (ADR-0063, widened to 24/60 by
// ADR-0106). Which one the picker OPENS on is `DEFAULT_IMPORT_WINDOW`, beside
// the operations both surfaces share.
const WINDOWS: { value: ImportWindow; label: MessageKey }[] = [
  { value: "3m", label: "backfill.window3m" },
  { value: "6m", label: "backfill.window6m" },
  { value: "12m", label: "backfill.window12m" },
  { value: "24m", label: "backfill.window24m" },
  { value: "60m", label: "backfill.window60m" },
];

// A run whose updated_at hasn't moved in this long is honestly "stuck", not
// "in progress" — the contract's own doc comment on BackfillStatus.updated_at
// calls this out ("a killed worker leaves this honest"). Long enough that
// ordinary poll jitter or a slow provider batch never false-positives, short
// enough that a genuinely dead worker surfaces within a couple of polls of
// the threshold rather than staying "live" indefinitely.
const STALE_AFTER_MS = 3 * 60_000;

// The contract pins v1 estimates to USD minor units and leaves `currency`
// optional, so USD is the documented fallback rather than a guess. Named
// identically in onboarding-backread.tsx, which renders THIS same field from
// the same preview: two screens holding two answers for one number means one of
// them is lying to the reader about what they are about to spend. The symbol
// itself is never spelled here — Intl derives it from the code and the locale.
const FALLBACK_CURRENCY = "USD";

// Both preview and start can answer connector_unsupported (a provider with no
// Backfiller — IMAP today) or window_narrowing (start only, a widen-only
// re-run) — pull the RFC 7807 code out of a thrown ProblemError so the render
// can branch to its own honest sentence instead of the raw server detail.
function errorCodeOf(error: unknown): string | null {
  return error instanceof ProblemError ? problemCode(error.problem) : null;
}

// connector_unsupported is a structural fact about the provider (no
// Backfiller behind it), independent of which window was picked — either op
// can be the one that surfaces it, depending on whether the setup screen's
// auto-preview or an explicit start round-trips first. window_narrowing only
// ever comes from start (preview never enqueues a run).
function classifyBackfillErrors(
  previewError: unknown,
  startError: unknown,
): { unsupported: boolean; narrowing: boolean } {
  const previewCode = errorCodeOf(previewError);
  const startCode = errorCodeOf(startError);
  return {
    unsupported:
      previewCode === "connector_unsupported" ||
      startCode === "connector_unsupported",
    narrowing: startCode === "window_narrowing",
  };
}

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

export function BackfillPanel({
  provider,
  initial,
}: {
  provider: Provider;
  // The run row already embedded in GET /connectors (CaptureConnection.
  // backfill) — seeds the first render so a live run shows immediately.
  initial?: BackfillStatus;
}) {
  const t = useT();
  const [skipped, setSkipped] = useState(false);
  const importRun = useBackfillRun({ provider, initial, previewHeld: skipped });
  const { status, preview, start, cancel, window, setWindow } = importRun;

  const { unsupported, narrowing } = classifyBackfillErrors(
    preview.error,
    start.error,
  );

  // This card draws the run in full, so the shell's capture chip stands down
  // while it is on screen rather than gauging the same import twice.
  useDrawsImportRun(isLiveRun(status.data?.state));

  if (skipped) {
    return (
      <p className="t-caption backfill-skipped">{t("backfill.skippedNote")}</p>
    );
  }
  if (status.isPending) {
    return <p className="t-caption">{t("backfill.loading")}</p>;
  }
  if (status.isError) {
    // The status read failing must not block the wizard — the nightly sweep
    // still runs; the user just loses the live view here.
    return <p className="t-caption">{t("backfill.statusUnavailable")}</p>;
  }

  const run = status.data;
  if (importRun.isSetup) {
    return (
      <BackfillSetup
        window={window}
        onWindowChange={setWindow}
        unsupported={unsupported}
        narrowing={narrowing}
        previewPending={preview.isPending}
        previewData={preview.data}
        previewErrorMessage={
          preview.isError ? problemMessageOf(preview.error, t) : null
        }
        startPending={start.isPending}
        startErrorMessage={
          start.isError ? problemMessageOf(start.error, t) : null
        }
        onStart={() => start.mutate(window)}
        onSkip={() => setSkipped(true)}
      />
    );
  }

  return (
    <RunView
      run={run}
      cancelling={cancel.isPending}
      cancelError={cancel.isError ? problemMessageOf(cancel.error, t) : null}
      onCancel={() => cancel.mutate()}
      onRestart={() => importRun.restart(run)}
    />
  );
}

// The window-picker + scope-preview + explicit-start setup screen, shown
// while no run has ever started. Split out of BackfillPanel so the several
// independent honest states here (loading the scope, a generic preview/start
// failure, a refused narrowing, and the connector_unsupported capability
// statement) don't all pile into one function's complexity budget.
function BackfillSetup({
  window,
  onWindowChange,
  unsupported,
  narrowing,
  previewPending,
  previewData,
  previewErrorMessage,
  startPending,
  startErrorMessage,
  onStart,
  onSkip,
}: {
  window: ImportWindow;
  onWindowChange: (w: ImportWindow) => void;
  unsupported: boolean;
  narrowing: boolean;
  previewPending: boolean;
  previewData: components["schemas"]["BackfillPreview"] | undefined;
  previewErrorMessage: string | null;
  startPending: boolean;
  startErrorMessage: string | null;
  onStart: () => void;
  onSkip: () => void;
}) {
  const t = useT();

  // A provider with no Backfiller behind it (IMAP today) can't run this op
  // at all, whichever window is picked — the honest answer is a capability
  // statement, not a retryable error inside the rest of the setup form.
  if (unsupported) {
    return (
      <div className="backfill-setup">
        <h3 className="backfill-h">
          <History aria-hidden /> {t("backfill.title")}
        </h3>
        <p className="t-caption backfill-unsupported">
          {t("backfill.unsupportedNote")}
        </p>
      </div>
    );
  }

  return (
    <div className="backfill-setup">
      <h3 className="backfill-h">
        <History aria-hidden /> {t("backfill.title")}
      </h3>
      <p className="t-caption">{t("backfill.intro")}</p>
      {/* The design system's radio GROUP, not a div wearing `role="radiogroup"`:
          the ARIA pair is the weaker spelling of what a `fieldset` and a
          `legend` say natively, and it was this screen's own wrapper to keep
          correct. The question stays off screen exactly as its `aria-label` was,
          so nothing visible changes. */}
      <ChoiceList
        className="backfill-windows"
        layout="row"
        legend={t("backfill.windowLabel")}
        hideLegend
        value={window}
        choices={WINDOWS.map((w) => ({ value: w.value, label: t(w.label) }))}
        onChange={onWindowChange}
      />
      {previewPending && !previewData && (
        <p className="t-caption">{t("backfill.previewLoading")}</p>
      )}
      {previewErrorMessage && (
        <p className="t-caption backfill-error">{previewErrorMessage}</p>
      )}
      {previewData && (
        <EstimateCard
          preview={previewData}
          starting={startPending}
          onStart={onStart}
        />
      )}
      {startErrorMessage && (
        <p className="t-caption backfill-error">
          {narrowing ? t("backfill.narrowingNote") : startErrorMessage}
        </p>
      )}
      {/* The one button on this panel that is not a Button — a bare <button>
          carrying a class from a sheet this file never imported, so on Settings
          it rendered as the browser's own grey chrome. */}
      <div className="backfill-foot">
        <Button small onClick={onSkip}>
          {t("backfill.skip")}
        </Button>
      </div>
    </div>
  );
}

// EstimateCard is the consent surface: the labeled estimate the user acts on.
function EstimateCard({
  preview,
  starting,
  onStart,
}: {
  preview: components["schemas"]["BackfillPreview"];
  starting: boolean;
  onStart: () => void;
}) {
  const t = useT();
  const { locale } = useLocale();
  const costMinor = preview.estimated_cost_minor ?? 0;
  return (
    <div className="backfill-estimate">
      <p>
        {t("backfill.estimateMessages")}{" "}
        <strong>~{formatNumber(preview.estimated_messages, locale)}</strong>
      </p>
      {costMinor > 0 && (
        <p className="t-caption">
          {t("backfill.estimateCost")} ~
          {formatMoney(
            costMinor,
            preview.currency ?? FALLBACK_CURRENCY,
            locale,
          )}
        </p>
      )}
      <p className="t-caption">{t("backfill.estimateNote")}</p>
      <Button
        variant="primary"
        pending={starting}
        busyLabel={t("backfill.starting")}
        onClick={onStart}
      >
        {t("backfill.startCta")}
      </Button>
    </div>
  );
}

// The three headline figures of a capture run — captured mail and the two
// record kinds it grows. Each is a live persisted-row count. While the run is
// still reading, the figure is a `CountUp`: a number still being earned climbs
// to where the poll put it instead of jumping there. Once the run has settled
// it is the plain number, because a figure the server has finished with has
// nothing left to count towards.
const CAPTURE_STATS: {
  key: "captured" | "people_created" | "organizations_created";
  label: MessageKey;
  icon: typeof Mail;
}[] = [
  { key: "captured", label: "backfill.statEmails", icon: Mail },
  { key: "people_created", label: "backfill.statPeople", icon: Users },
  {
    key: "organizations_created",
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

function RunView({
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
  // A percentage needs a denominator that is still true. The provider-side
  // count is a FLOOR — Gmail's exact count is capped at a page budget, and a
  // multi-year window reaches that cap far more often than a 12-month one —
  // so a run can scan past its own estimate. Clamping to 100% there would
  // show a full bar for an hour while the import kept going; the honest move
  // is the absolute counts this screen already falls back to when there is no
  // estimate at all, because at that moment there effectively is none. A run
  // that is not moving forward does not get a bar that implies otherwise.
  const denominator = run.estimated_messages ?? 0;
  const fraction =
    live && !stale && denominator > 0 && scanned <= denominator
      ? scanned / denominator
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
