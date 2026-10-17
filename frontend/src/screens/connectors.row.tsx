// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarDays, Mail, RefreshCw } from "lucide-react";
import { useId, useState } from "react";
import type { components } from "../api/schema";
import { Badge, Button, Modal, OverflowMenu } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { RowOpen } from "../design-system/rowopen";
import { SettingRow } from "../design-system/settingrow";
import { BackfillPanel } from "../features/import/backfill";
import { isLiveRun } from "../features/import/backfill-run";
import { isMailIcon, providerName } from "../format/connectorprovider";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import {
  errorClassKey,
  missingCalendarWriteGrant,
  missingSendGrant,
  statusLabel,
  statusTone,
} from "./connector-status";
import { ConnectionDrawer } from "./connectors.settings";

type CaptureConnection = components["schemas"]["CaptureConnection"];
type Provider = CaptureConnection["provider"];

// IMAP reconnects through its credential form; only these redirect to consent.
export const OAUTH_PROVIDERS: ReadonlySet<Provider> = new Set<Provider>([
  "gmail",
  "gcal",
  "graph",
  "graphcal",
]);

export function needsReconnect(conn: CaptureConnection): boolean {
  return (
    conn.status === "reauth_required" ||
    missingSendGrant(conn) ||
    missingCalendarWriteGrant(conn)
  );
}

// A badge shows only when something is wrong; the verbs keep a fixed trailing
// place on every row.
export function ConnectorRow({
  conn,
  connectPending,
  connectError,
  onReconnect,
  onDisconnect,
}: Readonly<{
  conn: CaptureConnection;
  // Scoped to this row: one mutation serves the card, and an unscoped flag
  // would draw every row's Reconnect busy.
  connectPending: boolean;
  connectError: string | null;
  onReconnect: () => void;
  onDisconnect: () => void;
}>) {
  const t = useT();
  const [opened, setOpened] = useState(false);
  const [importing, setImporting] = useState(false);
  const name = providerName(conn.provider, t);
  const identity = conn.account_label ? `${name} ${conn.account_label}` : name;
  const canImport = conn.status === "connected" && conn.can_backfill;
  return (
    <>
      <SettingRow
        testId={`connector-${conn.provider}`}
        label={
          <span className="connector-head">
            <span className="connector-id">
              {isMailIcon(conn.provider) ? (
                <Mail aria-hidden />
              ) : (
                <CalendarDays aria-hidden />
              )}
              <strong>{name}</strong>
            </span>
            <ConnectorBadges conn={conn} />
          </span>
        }
        description={<ConnectorCaption conn={conn} />}
        control={
          <>
            {needsReconnect(conn) && (
              <Button
                className="connector-reconnect"
                aria-label={t("connectors.reconnectNamed", { name: identity })}
                pending={connectPending}
                onClick={onReconnect}
              >
                <RefreshCw aria-hidden /> {t("connectors.reconnect")}
              </Button>
            )}
            <span className="connector-actions">
              <span className="cell-actions">
                <OverflowMenu label={t("table.rowActions", { name: identity })}>
                  {canImport && (
                    <Button
                      aria-haspopup="dialog"
                      onClick={() => setImporting(true)}
                    >
                      {t("backfill.title")}
                    </Button>
                  )}
                  <Button
                    variant="danger"
                    aria-haspopup="dialog"
                    onClick={onDisconnect}
                  >
                    {t("connectors.disconnect")}
                  </Button>
                </OverflowMenu>
              </span>
              <RowOpen
                label={t("connectors.openNamed", { name: identity })}
                opensDialog
                onOpen={() => setOpened(true)}
              />
            </span>
          </>
        }
      />
      {connectError && (
        <Callout
          tone="danger"
          kind="outcome"
          title={t("connectors.connectFailed")}
        >
          {connectError}
        </Callout>
      )}
      <ConnectionDrawer
        conn={conn}
        open={opened}
        onClose={() => setOpened(false)}
      />
      {/* Mounted only while open: the import fires a scope preview the moment
          it renders, and the roster must not spend one per mailbox. */}
      {importing && (
        <ImportDialog conn={conn} onClose={() => setImporting(false)} />
      )}
    </>
  );
}

function ConnectorBadges({ conn }: Readonly<{ conn: CaptureConnection }>) {
  const t = useT();
  const failing = conn.status === "error" || conn.status === "reauth_required";
  return (
    <>
      {failing && (
        <Badge tone={statusTone(conn.status)}>
          {t(statusLabel(conn.status))}
        </Badge>
      )}
      {missingCalendarWriteGrant(conn) && (
        <Badge tone="warning">{t("scheduling.readOnlyBadge")}</Badge>
      )}
      {missingSendGrant(conn) && (
        <Badge tone="warning">{t("connectors.cannotSend")}</Badge>
      )}
    </>
  );
}

function ConnectorCaption({ conn }: Readonly<{ conn: CaptureConnection }>) {
  const t = useT();
  const { locale } = useLocale();
  const failing = conn.status === "error" || conn.status === "reauth_required";
  return (
    <span className="connector-caption">
      {conn.account_label && (
        <span className="connector-account">{conn.account_label}</span>
      )}
      <span>
        {conn.last_synced_at
          ? t("connectors.lastSynced", {
              at: formatDateTime(conn.last_synced_at, locale, viewerZone()),
            })
          : t("connectors.neverSynced")}
      </span>
      {isLiveRun(conn.backfill?.state) && (
        <span>{t("backfill.runningTitle")}</span>
      )}
      {failing && (
        <ErrorLine inline standing>
          {t(errorClassKey(conn.last_sync_error_class))}
        </ErrorLine>
      )}
    </span>
  );
}

function ImportDialog({
  conn,
  onClose,
}: Readonly<{ conn: CaptureConnection; onClose: () => void }>) {
  const t = useT();
  const titleId = useId();
  const taskId = useId();
  return (
    <Modal
      open
      onClose={onClose}
      labelledBy={`${taskId} ${titleId}`}
      intent="form"
    >
      {/* The panel draws this heading itself, and only once it has loaded. */}
      <span id={taskId} className="sr-only">
        {t("backfill.title")}
      </span>
      <Heading size="large" id={titleId} className="t-h2 modal-title">
        {conn.account_label ?? providerName(conn.provider, t)}
      </Heading>
      <BackfillPanel provider={conn.provider} initial={conn.backfill} />
    </Modal>
  );
}
