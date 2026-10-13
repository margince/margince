// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { useDrawsImportRun } from "../app/import-onscreen";
import { Badge, Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Meter } from "../design-system/readings";
import { formatDuration, formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  IMPORT_WINDOW_LABELS,
  isImportWindow,
} from "../mail-history/window-picker";
import { EstimateCard } from "./backfill";
import { type ImportWindow, useBackfillRun } from "./backfill-run";
import { canResume, runReading } from "./backfillrunview";
import { problemMessageOf } from "./common";

// The Past email service of a connected mailbox: the import of mail from before
// it was connected. A connect asks for a window; a small one starts on its own,
// and a larger one, or one the server could not count, is held for the reader
// and lands here with its estimate and one Start import.

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

/** What the Past email service says: its name, its badge and verb, and what
 *  it reports under its sentence. The account's service list draws it. */
export type PastEmailParts = Readonly<{
  name: string;
  side: ReactNode;
  body?: ReactNode;
}>;

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
  // The scope is counted only for a held window: a row that merely offers the
  // import must not spend a provider round trip on every page load.
  const importRun = useBackfillRun({
    provider: mail.provider,
    initial: mail.backfill,
    previewHeld: heldWindow === null,
    initialWindow: heldWindow ?? undefined,
  });
  const run = importRun.status.data;
  useDrawsImportRun(run !== undefined && runReading(run).live);

  // A reconnect can hold a wider window over an import that already finished,
  // so the hold stands until a run of that window is moving or has run.
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
  const item = (side: ReactNode, body?: ReactNode) => ({ name, side, body });

  if (held !== null) {
    return item(
      <Badge tone="info">{t("connectors.svc.waiting")}</Badge>,
      <HeldImport held={held} run={run} importRun={importRun} />,
    );
  }
  if (!run || run.state === "none") {
    return item(
      <>
        <Badge>{t("connectors.svc.notImported")}</Badge>
        <Button onClick={onImport}>{t("connectors.svc.importPast")}</Button>
      </>,
    );
  }

  return item(
    <RunSide run={run} importRun={importRun} />,
    <RunBody run={run} importRun={importRun} />,
  );
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

// The badge and the verb of a run that has started: Stop while it can move,
// Continue on one that stopped on an error and kept its place.
function RunSide({
  run,
  importRun,
}: Readonly<{ run: BackfillStatus; importRun: ImportRun }>) {
  const t = useT();
  const { live, stale, reading } = runReading(run);
  let badge = <Badge tone="success">{t("connectors.svc.imported")}</Badge>;
  if (reading) {
    badge = <Badge tone="ai">{t("backfill.readingBadge")}</Badge>;
  } else if (live) {
    badge = stale ? (
      <Badge tone="warning">{t("connectors.health.attention")}</Badge>
    ) : (
      <Badge tone="info">{t("connectors.svc.queued")}</Badge>
    );
  } else if (run.state === "error") {
    badge = <Badge tone="warning">{t("connectors.svc.stopped")}</Badge>;
  } else if (run.state === "cancelled") {
    badge = <Badge>{t("connectors.svc.stopped")}</Badge>;
  }
  return (
    <>
      {/* The state word is announced when it changes; count ticks are not. */}
      <span role="status">{badge}</span>
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

// How far a run got: a bar against an exact estimate, the count alone against
// a floor (Gmail stops counting a large mailbox), and what it brought in.
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
