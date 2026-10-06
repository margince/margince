// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { useCan, useCanWrite } from "../app/capability";
import { useInstallationSettings } from "../app/uploadlimit";
import { Callout } from "../design-system/callout";
import { NumberSettingRow } from "../design-system/numbersetting";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { SettingList } from "../design-system/settingrow";
import { useT } from "../i18n";
import { problemMessageOf, QueryGate } from "./common";
import { useUpdateInstallationSettings } from "./installation-settings";

type Operations = components["schemas"]["OperationSettings"];

// The bounds the API refuses past, mirrored so a value out of range is refused
// in the box before the request. Keyed by the wire property, which is how
// backend/gates/settingbounds_test.go holds each to the contract. The two
// sweeps that may be switched off start at 0; the server refuses 1 to 299.
const OPERATION_BOUNDS = {
  agent_runner_interval_seconds: { min: 10, max: 3_600 },
  webhook_retry_interval_seconds: { min: 10, max: 3_600 },
  time_scan_interval_seconds: { min: 60, max: 86_400 },
  close_date_sweep_interval_seconds: { min: 3_600, max: 604_800 },
  follow_up_reconcile_interval_seconds: { min: 3_600, max: 604_800 },
  retention_sweep_interval_seconds: { min: 3_600, max: 604_800 },
  geocode_backfill_interval_seconds: { min: 0, max: 604_800 },
  technical_backfill_interval_seconds: { min: 0, max: 604_800 },
  gmail_watch_scan_interval_seconds: { min: 600, max: 86_400 },
  graph_watch_scan_interval_seconds: { min: 600, max: 86_400 },
  gmail_watch_renew_within_hours: { min: 24, max: 144 },
  graph_watch_renew_within_hours: { min: 24, max: 60 },
  send_rate_limit: { min: 1, max: 1_000 },
  send_rate_window_seconds: { min: 10, max: 3_600 },
  send_max_age_hours: { min: 1, max: 168 },
} as const satisfies Record<keyof Operations, { min: number; max: number }>;

type OperationRow = Readonly<{
  property: keyof Operations;
  copy:
    | "agentRunner"
    | "webhookRetry"
    | "timeScan"
    | "closeDate"
    | "followUp"
    | "retention"
    | "geocode"
    | "technical"
    | "gmailWatchScan"
    | "graphWatchScan"
    | "gmailWatchRenew"
    | "graphWatchRenew"
    | "sendRateLimit"
    | "sendRateWindow"
    | "sendMaxAge";
}>;

// Quickest pass first, then the daily ones, then the two mail subscriptions
// with the margin each is renewed by.
const SCHEDULE_ROWS: readonly OperationRow[] = [
  { property: "agent_runner_interval_seconds", copy: "agentRunner" },
  { property: "webhook_retry_interval_seconds", copy: "webhookRetry" },
  { property: "time_scan_interval_seconds", copy: "timeScan" },
  { property: "close_date_sweep_interval_seconds", copy: "closeDate" },
  { property: "follow_up_reconcile_interval_seconds", copy: "followUp" },
  { property: "retention_sweep_interval_seconds", copy: "retention" },
  { property: "geocode_backfill_interval_seconds", copy: "geocode" },
  { property: "technical_backfill_interval_seconds", copy: "technical" },
  { property: "gmail_watch_scan_interval_seconds", copy: "gmailWatchScan" },
  { property: "gmail_watch_renew_within_hours", copy: "gmailWatchRenew" },
  { property: "graph_watch_scan_interval_seconds", copy: "graphWatchScan" },
  { property: "graph_watch_renew_within_hours", copy: "graphWatchRenew" },
];

const PACING_ROWS: readonly OperationRow[] = [
  { property: "send_rate_limit", copy: "sendRateLimit" },
  { property: "send_rate_window_seconds", copy: "sendRateWindow" },
  { property: "send_max_age_hours", copy: "sendMaxAge" },
];

// One card over a slice of the installation's operating values. The page opens
// on `job_health:read`, which a custom role can hold without
// `installation_settings:read`; a card narrower than its page withholds itself
// rather than drawing a refusal the reader can do nothing about.
function OperationsCard({
  title,
  sub,
  rows,
}: Readonly<{ title: string; sub: string; rows: readonly OperationRow[] }>) {
  const t = useT();
  const canRead = useCan("installation_settings", "read");
  const canManage = useCanWrite("installation_settings", "update");
  const query = useInstallationSettings(canRead);
  const update = useUpdateInstallationSettings(() => undefined);
  if (!canRead) {
    return null;
  }
  return (
    <Panel title={title}>
      <PanelBody className="form-stack">
        <PanelIntro>{sub}</PanelIntro>
        {!canManage && <PanelIntro>{t("operations.adminOnly")}</PanelIntro>}
        <QueryGate query={query} pendingLabel={title}>
          {(settings) => (
            <SettingList>
              {rows.map((row) => (
                <NumberSettingRow
                  key={row.property}
                  label={t(`operations.${row.copy}.label`)}
                  description={t(`operations.${row.copy}.help`)}
                  testId={`operation-${row.property}`}
                  value={settings.operations[row.property]}
                  {...OPERATION_BOUNDS[row.property]}
                  refusal={t("operations.refusal")}
                  disabled={!canManage || update.isPending}
                  onCommit={(next) => update.mutate({ [row.property]: next })}
                />
              ))}
            </SettingList>
          )}
        </QueryGate>
        {update.isError && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("operations.updateFailed")}
          >
            {problemMessageOf(update.error, t)}
          </Callout>
        )}
      </PanelBody>
    </Panel>
  );
}

// How often each background pass runs, and how far ahead a mail subscription
// is renewed.
export function BackgroundSchedulesCard() {
  const t = useT();
  return (
    <OperationsCard
      title={t("operations.schedules.title")}
      sub={t("operations.schedules.sub")}
      rows={SCHEDULE_ROWS}
    />
  );
}

// How fast one mailbox may send. Beside the schedules because it is the same
// kind of answer: how hard the worker leans on somebody else's service.
export function SendPacingCard() {
  const t = useT();
  return (
    <OperationsCard
      title={t("operations.pacing.title")}
      sub={t("operations.pacing.sub")}
      rows={PACING_ROWS}
    />
  );
}
