import { type ReactNode, useId } from "react";
import type { components } from "../../api/schema";
import { Button, Modal } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody } from "../../design-system/panel";
import { formatMoney, formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import {
  type ImportWindow,
  isLiveRun,
  useBackfillRun,
} from "../../lib/backfill-run";
import { useDrawsImportRun } from "../../lib/import-onscreen";
import { ProblemError, problemCode, problemMessageOf } from "../../lib/problem";
import {
  IMPORT_WINDOW_LABELS,
  ImportWindowPicker,
} from "../../mail-history/window-picker";
import { RunView } from "./backfillrunview";
import "../../components/backfill.css";

// The bounded connect-time backfill (ADR-0063). Pick a window, see the scope
// before anything spends (the estimate card is the consent surface), then watch
// real progress. Every number rendered here is a persisted-row count from the
// single-row status read. The scope preview auto-loads, but the spend waits for
// the explicit "Start the import" consent.
//
// It is the body of the Settings import dialog, opened from a connected
// account's Past email service. The account row seeds it with the run row
// embedded in `CaptureConnection.backfill`, so a live run shows with no extra
// request. Without a seed it fetches on mount.

type BackfillStatus = components["schemas"]["BackfillStatus"];
type Provider = components["schemas"]["CaptureConnection"]["provider"];

// The contract pins v1 estimates to USD minor units and leaves `currency`
// optional, so USD is the documented fallback rather than a guess. Named
// identically in onboarding-backread.tsx, which renders THIS same field from
// the same preview: two screens holding two answers for one number means one of
// them is lying to the reader about what they are about to spend. The symbol
// itself is never spelled here: Intl derives it from the code and the locale.
const FALLBACK_CURRENCY = "USD";

// Both preview and start can answer connector_unsupported (a provider with no
// Backfiller: IMAP today) or window_narrowing (start only, a widen-only
// re-run). Pull the RFC 7807 code out of a thrown ProblemError so the render
// can branch to its own sentence instead of the raw server detail.
function errorCodeOf(error: unknown): string | null {
  return error instanceof ProblemError ? problemCode(error.problem) : null;
}

// connector_unsupported is a structural fact about the provider, independent of
// the window picked. Either op can surface it, depending on whether the
// auto-preview or an explicit start round-trips first. window_narrowing only
// ever comes from start, because preview never enqueues a run.
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

/**
 * "Import older email" for one connected mailbox: the pick, the scope and the
 * start, or the last run with its own verbs. The panel mounts only while the
 * dialog is open, because its setup counts the scope the moment it renders.
 */
export function ImportPastEmailDialog({
  provider,
  initial,
  baseline,
  open,
  onClose,
}: Readonly<{
  provider: Provider;
  initial?: BackfillStatus;
  baseline?: ReactNode;
  open: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const headingId = useId();
  return (
    <Modal open={open} onClose={onClose} labelledBy={headingId} intent="form">
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {t("connectors.menu.importOlder")}
      </Heading>
      {open && (
        <BackfillPanel
          provider={provider}
          initial={initial}
          baseline={baseline}
          onClose={onClose}
        />
      )}
    </Modal>
  );
}

export function BackfillPanel({
  provider,
  initial,
  onClose,
  baseline,
}: {
  provider: Provider;
  // The run row already embedded in GET /connectors (CaptureConnection.
  // backfill): seeds the first render so a live run shows immediately.
  initial?: BackfillStatus;
  /** Leaves the panel: "Not now", or a start the server accepted, after which
   *  the account row is where the run is watched. */
  onClose: () => void;
  /** The mailbox's history-baseline control, offered beside the window. */
  baseline?: ReactNode;
}) {
  const t = useT();
  const importRun = useBackfillRun({ provider, initial });
  const { status, preview, start, cancel, window, setWindow } = importRun;

  const { unsupported, narrowing } = classifyBackfillErrors(
    preview.error,
    start.error,
  );

  // This card draws the run in full, so the shell's capture chip stands down
  // while it is on screen rather than gauging the same import twice.
  useDrawsImportRun(isLiveRun(status.data?.state));

  if (status.isPending) {
    return <p>{t("backfill.loading")}</p>;
  }
  if (status.isError) {
    // The status read failing must not block the wizard: the nightly sweep
    // still runs; the user just loses the live view here.
    return <p>{t("backfill.statusUnavailable")}</p>;
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
        offeredWindows={run?.offered_windows}
        previewErrorMessage={
          preview.isError ? problemMessageOf(preview.error, t) : null
        }
        startPending={start.isPending}
        startErrorMessage={
          start.isError ? problemMessageOf(start.error, t) : null
        }
        onStart={() => importRun.begin(window, onClose)}
        onSkip={onClose}
        baseline={baseline}
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
      onResume={() => importRun.resume(run)}
      resuming={start.isPending}
      resumeError={start.isError ? problemMessageOf(start.error, t) : null}
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
  offeredWindows,
  previewErrorMessage,
  startPending,
  startErrorMessage,
  onStart,
  onSkip,
  baseline,
}: {
  window: ImportWindow;
  onWindowChange: (w: ImportWindow) => void;
  unsupported: boolean;
  narrowing: boolean;
  previewPending: boolean;
  previewData: components["schemas"]["BackfillPreview"] | undefined;
  /** The windows this installation admits; absent means the whole set. */
  offeredWindows: readonly ImportWindow[] | undefined;
  previewErrorMessage: string | null;
  startPending: boolean;
  startErrorMessage: string | null;
  onStart: () => void;
  onSkip: () => void;
  baseline?: ReactNode;
}) {
  const t = useT();

  // A provider with no Backfiller behind it (IMAP today) can't run this op at
  // all, whichever window is picked. The answer is a capability statement, not
  // a retryable error inside the rest of the setup form.
  if (unsupported) {
    return (
      <Panel>
        <PanelBody>
          <p>{t("backfill.unsupportedNote")}</p>
        </PanelBody>
      </Panel>
    );
  }

  return (
    <Panel actions={<Button onClick={onSkip}>{t("backfill.skip")}</Button>}>
      <PanelBody className="form-stack">
        <p>{t("backfill.intro")}</p>
        <ImportWindowPicker
          value={window}
          onChange={onWindowChange}
          preview={previewData}
          offered={offeredWindows}
        />
        <p className="t-caption">{t("backfill.extendNote")}</p>
        {baseline}
        {previewErrorMessage && <ErrorLine>{previewErrorMessage}</ErrorLine>}
        <EstimateCard
          preview={previewData}
          window={window}
          counting={previewPending && !previewData}
          starting={startPending}
          onStart={onStart}
        />
        {startErrorMessage && (
          <ErrorLine>
            {narrowing ? t("backfill.narrowingNote") : startErrorMessage}
          </ErrorLine>
        )}
      </PanelBody>
    </Panel>
  );
}

// EstimateCard is the consent surface. It shows whatever is known about the
// scope, and the verb that acts on it.
//
// The verb stands in every scope state: counted, still counting, or refused.
// The window bounds the run and the estimate only describes it. A slow or
// failed count is a reason to say so, never to withhold the import.
export function EstimateCard({
  preview,
  window,
  counting,
  starting,
  onStart,
}: {
  preview: components["schemas"]["BackfillPreview"] | undefined;
  /** The period the reader picked: the scope the card leads with. */
  window: ImportWindow;
  /** No estimate for this window yet and one is on its way. A refused estimate
   *  is neither: its sentence is already above the card. */
  counting: boolean;
  starting: boolean;
  onStart: () => void;
}) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const costMinor = preview?.estimated_cost_minor;
  return (
    <div className="backfill-estimate">
      {counting && <p>{t("backfill.previewLoading")}</p>}
      {/* THE WINDOW FIRST. What the mailbox owner consents to is a period of their own
          mailbox; the count describes that period and is not the thing being
          agreed to. It also degrades better: the scope sentence is true while
          the count is still arriving, or when it never does. */}
      <p>
        {t("backfill.scopeIs", { window: t(IMPORT_WINDOW_LABELS[window]) })}
      </p>
      {/* The count is its own line and its own condition. `estimated_messages`
          is required on the wire, so an answer without it is a server too old
          to send one or a response nothing routed: and the scope sentence
          above is still true in both cases, which is the whole reason the
          window leads. Rendering a count nobody produced is how `~undefined`
          reaches a consent surface. */}
      {typeof preview?.estimated_messages === "number" && (
        <p className="t-caption">
          {/* Selected on the RAW count and printed with the formatted one: a
              mailbox with a single message in the window is a real answer, and
              "1 messages" is the sentence a reader trusts least. */}
          {plural(
            preview.estimate_is_floor
              ? "backfill.estimateMessagesAtLeast"
              : "backfill.estimateMessagesExact",
            preview.estimated_messages,
            { count: formatNumber(preview.estimated_messages, locale) },
          )}
        </p>
      )}
      {preview && costMinor !== undefined && (
        <p className="t-caption">
          {t("backfill.estimateCost", {
            cost: formatMoney(
              costMinor,
              preview.currency ?? FALLBACK_CURRENCY,
              locale,
            ),
          })}
        </p>
      )}
      {preview?.estimate_is_floor && costMinor !== undefined && (
        <p className="t-caption">{t("backfill.costFloorNote")}</p>
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
