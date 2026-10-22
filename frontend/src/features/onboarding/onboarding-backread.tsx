// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Skeleton } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { formatMoney, formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  type ImportWindow,
  isLiveRun,
  useBackfillRun,
} from "../../lib/backfill-run";
import { useDrawsImportRun } from "../../lib/import-onscreen";
import { problemMessageOf } from "../../lib/problem";
import { ImportWindowPicker } from "../../mail-history/window-picker";
import "../../components/backfill.css";
import "./onboarding-backread.css";
import { BackreadRun } from "./onboarding-backread.run";

/**
 * The last act of onboarding: a mailbox is connected, so the flow asks how far
 * back to read it. It starts the read and shows what it found.
 *
 * Connecting and reading are two operations. The scope is previewed before the
 * spend (ADR-0020), and the step footer's "Import {period} and continue"
 * starts the read, so the page has one primary.
 *
 * `BackfillStatus` requires only `state`, so a tally with no count is
 * omitted, never drawn as a 0. Progress is a fraction only when the server gave
 * `estimated_messages`. The run is a server-side job, so leaving is always
 * available.
 */

export type BackfillStatus = components["schemas"]["BackfillStatus"];

type BackfillPreview = components["schemas"]["BackfillPreview"];
type Provider = components["schemas"]["CaptureConnection"]["provider"];

// The contract pins v1 estimates to USD minor units and leaves `currency`
// optional, so USD is the documented fallback rather than a guess. The symbol
// itself is never spelled here: Intl derives it from the code and the locale.
const FALLBACK_CURRENCY = "USD";

// The template-ready `{detail}` value for a mutation that may or may not have
// failed: `null` while it hasn't, the failure's reader-safe text once it has.
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

/**
 * The import choice for one connected mailbox, shared by the panel that draws
 * it and the footer that starts it. `initial` is the run row embedded in the
 * `GET /connectors` roster row. Returning to a read in progress shows it at
 * once and never offers to start a second one.
 */
export function useOnboardingBackread({
  provider,
  initial,
}: Readonly<{ provider: Provider; initial?: BackfillStatus }>) {
  const importRun = useBackfillRun({ provider, initial });
  // This step draws the run in full, so the shell's capture chip stands down
  // for as long as it is on screen rather than gauging the same import twice.
  useDrawsImportRun(isLiveRun(importRun.status.data?.state));
  return importRun;
}

export type BackreadChoice = ReturnType<typeof useOnboardingBackread>;

/** Whether the window pick is what the reader faces, so the way on is the
 *  import. A run already started, or a status nobody could read, leaves plain
 *  continue: capture itself is unaffected either way. */
export function offersImport(choice: BackreadChoice): boolean {
  return choice.status.data !== undefined && choice.isSetup;
}

export function BackreadPanel({
  choice,
}: Readonly<{ choice: BackreadChoice }>) {
  const t = useT();
  const { status, preview, start, cancel } = choice;
  const selected = choice.window;

  // `preview.data`/`preview.error` are the mutation's last result. It outlives a
  // change of `selected` to a window nobody has previewed. `preview.variables`
  // names the window that result belongs to, so a stale estimate is withheld.
  // Start does not wait on it. The window is the consent and the estimate only
  // describes it. A verb held shut while a large mailbox is counted reads as a
  // broken button.
  const previewForSelection = preview.variables === selected;
  const previewSettled = previewForSelection && !preview.isPending;

  if (status.data === undefined) {
    return status.isError ? (
      <ErrorLine>{t("backfill.statusUnavailable")}</ErrorLine>
    ) : (
      <div className="ob-backread">
        <Skeleton width="70%" />
        <Skeleton width="45%" />
      </div>
    );
  }

  if (choice.isSetup) {
    return (
      <BackreadSetup
        selected={selected}
        onSelect={choice.setWindow}
        // Gated on settled, not just on-selection: the old window's estimate
        // stays withheld on our own terms, whatever react-query does with
        // `data` while a mutation is pending.
        preview={previewSettled ? preview.data : undefined}
        previewProblem={
          previewForSelection
            ? safeDetail(preview.isError, preview.error, t)
            : null
        }
        counting={!previewSettled}
        startProblem={safeDetail(start.isError, start.error, t)}
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
      onRestart={() => choice.restart(run)}
    />
  );
}

// The window pick and the consent it carries. The read-only note closes the
// panel, right above the footer button that acts on it. A reassurance read
// after the click is a reassurance nobody read.
function BackreadSetup({
  selected,
  onSelect,
  preview,
  previewProblem,
  counting,
  startProblem,
}: Readonly<{
  selected: ImportWindow;
  onSelect: (pick: ImportWindow) => void;
  preview: BackfillPreview | undefined;
  previewProblem: string | null;
  /** THIS selection's estimate has not settled yet (found or failed), so the
   *  scope is still being counted. A sentence, never a hold on the start. */
  counting: boolean;
  startProblem: string | null;
}>) {
  const t = useT();

  return (
    <section className="ob-backread">
      <ImportWindowPicker
        value={selected}
        onChange={onSelect}
        preview={preview}
      />
      <BackreadScope
        preview={preview}
        problem={previewProblem}
        counting={counting}
      />
      <p>{t("ob.backread.note")}</p>
      {startProblem !== null && (
        <ErrorLine>
          {t("ob.backread.startFailed", { detail: startProblem })}
        </ErrorLine>
      )}
    </section>
  );
}

// What the selected window would touch. An estimate still being counted, and an
// estimator that failed, are each stated here and neither stands in the way of
// the start: not knowing the size of the mailbox yet is a reason to say so,
// never a reason to refuse the read.
function BackreadScope({
  preview,
  problem,
  counting,
}: Readonly<{
  preview: BackfillPreview | undefined;
  problem: string | null;
  counting: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // Absent when no model rate applied to this window: an unpriced estimate
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
    <div className="backfill-estimate ob-backread-scope" aria-live="polite">
      {counting && <p>{t("ob.backread.estimating")}</p>}
      {preview && (
        <p className="ob-backread-estimate">
          {t(
            preview.estimate_is_floor
              ? "ob.backread.estimateAtLeast"
              : "ob.backread.estimate",
            {
              messages: formatNumber(preview.estimated_messages, locale),
            },
          )}
        </p>
      )}
      {preview?.estimate_quality === "heuristic" && (
        <p className="t-caption">{t("ob.backread.estimateHeuristic")}</p>
      )}
      {cost !== null && (
        <p className="ob-backread-cost">
          {t("ob.backread.estimateCost", { cost })}
        </p>
      )}
      {preview?.estimate_is_floor && cost !== null && (
        <p className="t-caption">{t("backfill.costFloorNote")}</p>
      )}
      {problem !== null && (
        <ErrorLine>
          {t("ob.backread.estimateFailed", { detail: problem })}
        </ErrorLine>
      )}
    </div>
  );
}
