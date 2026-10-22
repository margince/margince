// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Meter } from "../../design-system/readings";
import type { ServiceRowTone } from "../../design-system/servicerow";
import { type StatusKind, statusTone } from "../../design-system/statusbadge";
import { formatDuration, formatNumber } from "../../format/format";
import { type Translator, useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { type ImportWindow, useBackfillRun } from "../../lib/backfill-run";
import { useDrawsImportRun } from "../../lib/import-onscreen";
import { problemMessageOf } from "../../lib/problem";
import {
  IMPORT_WINDOW_LABELS,
  isImportWindow,
} from "../../mail-history/window-picker";
import { EstimateCard } from "./backfill";
import { canResume, runReading } from "./backfillrunview";

// The Past email service of a connected mailbox: the import of mail from before
// it was connected. A connect asks for a window. A small one starts on its own.
// A larger one, or one the server could not count, is held for the reader and
// lands here with its estimate and one Start import.

type CaptureConnection = components["schemas"]["CaptureConnection"];
type BackfillStatus = components["schemas"]["BackfillStatus"];

const STATS = [
  { key: "captured", label: "backfill.statEmails" },
  { key: "contacts_created", label: "backfill.statContacts" },
  { key: "companies_created", label: "backfill.statCompanies" },
] as const satisfies readonly Readonly<{
  key: keyof NonNullable<BackfillStatus["counts"]>;
  label: MessageKey;
}>[];

type PastEmailStatus = Readonly<{
  tone: ServiceRowTone;
  label: string;
  live?: boolean;
}>;

/** What the Past email service says: its name, its one status and verb, and
 *  what it reports under its sentence. The account's service list draws it. */
export type PastEmailParts = Readonly<{
  name: string;
  status: PastEmailStatus;
  action?: ReactNode;
  body?: ReactNode;
}>;

function pastStatus(kind: StatusKind, label: string, live?: boolean) {
  return { tone: statusTone(kind), label, live };
}

export function usePastEmail({
  mail,
  heldWindow,
  onImport,
}: Readonly<{
  mail: CaptureConnection;
  /** A window asked for at connect that waits for the reader, from the
   *  landing; null when none is held. */
  heldWindow: ImportWindow | null;
  /** Opens the import dialog: the pick, the scope and the start. */
  onImport: () => void;
}>): PastEmailParts {
  const t = useT();
  // The scope is counted only for a held window. A row that merely offers the
  // import must not spend a provider round trip on every page load.
  const importRun = useBackfillRun({
    provider: mail.provider,
    initial: mail.backfill,
    previewHeld: heldWindow === null,
    initialWindow: heldWindow ?? undefined,
  });
  const run = importRun.status.data;
  useDrawsImportRun(run !== undefined && runReading(run).live);

  // A reconnect can hold a wider window over an import that already finished.
  // The hold stands until a run of that window is moving or has run.
  const covered =
    run !== undefined && (runReading(run).live || run.window === heldWindow);
  const held = covered ? null : heldWindow;
  const ran = run && run.state !== "none" ? run.window : null;
  const window = held ?? ran;
  const name = isImportWindow(window)
    ? t("connectors.svc.pastEmailWindow", {
        window: t(IMPORT_WINDOW_LABELS[window]),
      })
    : t("connectors.svc.pastEmail");

  if (held !== null) {
    return {
      name,
      status: pastStatus("attention", t("connectors.svc.waiting")),
      body: <HeldImport held={held} run={run} importRun={importRun} />,
    };
  }
  if (!run || run.state === "none") {
    return {
      name,
      status: pastStatus("neutral", t("connectors.svc.notImported")),
      action: (
        <Button onClick={onImport}>{t("connectors.svc.importPast")}</Button>
      ),
    };
  }
  const status = runStatus(run, t);
  return {
    name,
    status,
    action: <RunVerbs run={run} importRun={importRun} />,
    body: (
      <>
        {/* The state word is announced when it changes; count ticks are not. */}
        <span role="status" className="sr-only">
          {status.label}
        </span>
        <RunBody run={run} importRun={importRun} />
      </>
    ),
  };
}

type ImportRun = ReturnType<typeof useBackfillRun>;

// A window asked for at connect that did not start by itself: its count and
// one Start.
function HeldImport({
  held,
  run,
  importRun,
}: Readonly<{
  held: ImportWindow;
  run: BackfillStatus | undefined;
  importRun: ImportRun;
}>) {
  const t = useT();
  const { preview, start } = importRun;
  // Over a run that stopped and kept its place, the held window reads the
  // mailbox again from the top rather than continuing that run.
  const startOver = run !== undefined && canResume(run);
  return (
    <div className="connection-service-held">
      <p className="t-caption">{t("connectors.svc.heldNote")}</p>
      {preview.isError && (
        <ErrorLine>{problemMessageOf(preview.error, t)}</ErrorLine>
      )}
      <EstimateCard
        preview={preview.data}
        window={held}
        counting={preview.isPending && !preview.data}
        starting={start.isPending}
        onStart={() => start.mutate({ window: held, startOver })}
      />
      {start.isError && (
        <ErrorLine>{problemMessageOf(start.error, t)}</ErrorLine>
      )}
    </div>
  );
}

// A run that has started, as one status: reading is running work (grey, live),
// a stall or an error someone must look at is amber.
function runStatus(run: BackfillStatus, t: Translator): PastEmailStatus {
  const { live, stale, reading } = runReading(run);
  if (reading) return pastStatus("neutral", t("backfill.readingBadge"), true);
  if (live && stale) {
    return pastStatus("attention", t("connectors.health.attention"));
  }
  if (live) return pastStatus("neutral", t("connectors.svc.queued"));
  if (run.state === "error") {
    return pastStatus("attention", t("connectors.svc.stopped"));
  }
  if (run.state === "cancelled") {
    return pastStatus("neutral", t("connectors.svc.stopped"));
  }
  return pastStatus("done", t("connectors.svc.imported"));
}

// The verb of a run that has started: Stop while it can move, Continue on one
// that stopped on an error and kept its place.
function RunVerbs({
  run,
  importRun,
}: Readonly<{ run: BackfillStatus; importRun: ImportRun }>) {
  const t = useT();
  const { live } = runReading(run);
  return (
    <>
      {live && (
        <Button
          disabled={importRun.cancel.isPending}
          onClick={() => importRun.cancel.mutate()}
        >
          {t("backfill.cancel")}
        </Button>
      )}
      {canResume(run) && (
        <Button
          pending={importRun.start.isPending}
          busyLabel={t("backfill.starting")}
          onClick={() => importRun.resume(run)}
        >
          {t("backfill.resumeCta")}
        </Button>
      )}
    </>
  );
}

// How far a run got. A bar shows against an exact estimate. The count alone
// shows against a floor (Gmail stops counting a large mailbox). Then comes what
// it brought in.
function RunBody({
  run,
  importRun,
}: Readonly<{ run: BackfillStatus; importRun: ImportRun }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const { scanned, live, stale, agoMs, fraction } = runReading(run);
  const total = run.estimated_messages;
  const metered =
    fraction !== null && run.estimate_is_floor !== true && total != null;
  let progress = plural("backfill.countScanned", scanned, {
    count: formatNumber(scanned, locale),
  });
  if (metered) {
    progress = plural("connectors.svc.checked", total, {
      done: formatNumber(scanned, locale),
      count: formatNumber(total, locale),
    });
  } else if (stale) {
    progress = t("backfill.staleUpdated", {
      duration: formatDuration(agoMs, locale),
    });
  }
  return (
    <>
      {live && (
        <div className="connection-service-progress">
          {metered && (
            <Meter
              value={scanned}
              max={total}
              label={t("backfill.progressLabel")}
              dense
            />
          )}
          <span className="t-caption">{progress}</span>
        </div>
      )}
      <span className="connection-service-facts">
        {STATS.map((stat) => (
          <span key={stat.key}>
            {t(stat.label)}{" "}
            <b>{formatNumber(run.counts?.[stat.key] ?? 0, locale)}</b>
          </span>
        ))}
      </span>
      {run.state === "error" && (
        <ErrorLine>
          {t(
            canResume(run) ? "backfill.errorResumeNote" : "backfill.errorNote",
          )}
        </ErrorLine>
      )}
      {run.state === "cancelled" && (
        <span className="t-caption">{t("backfill.cancelledNote")}</span>
      )}
      {importRun.cancel.isError && (
        <ErrorLine>{problemMessageOf(importRun.cancel.error, t)}</ErrorLine>
      )}
      {importRun.start.isError && (
        <ErrorLine>{problemMessageOf(importRun.start.error, t)}</ErrorLine>
      )}
    </>
  );
}
