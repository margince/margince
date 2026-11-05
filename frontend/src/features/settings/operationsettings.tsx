// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useRef } from "react";
import type { components } from "../../api/schema";
import { Disclosure } from "../../design-system/atoms";
import { NumberSettingRow } from "../../design-system/numbersetting";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { PanelNotices } from "../../design-system/panelnotices";
import { QueryGate } from "../../design-system/querystates";
import { useSavePage } from "../../design-system/savepage";
import { SettingList } from "../../design-system/settingrow";
import { boundsText } from "../../format/bounds";
import { formatNumber } from "../../format/format";
import { formatInterval } from "../../format/interval";
import { lockReasonOr } from "../../format/settinglocks";
import { useLocale, usePlural, useT, type Words } from "../../i18n";
import { useCan, useCanWrite } from "../../lib/capability";
import { useHealthVisible } from "../../lib/healthscope";
import { useInstallationSettings } from "../../lib/uploadlimit";
import { useUrlParams } from "../../lib/urlstate";
import { useUpdateInstallationSettings } from "../sign-in/installation-settings";

type Operations = components["schemas"]["OperationSettings"];
type InstallationSettings = components["schemas"]["InstallationSettings"];

// The bounds the API refuses past, mirrored so a value out of range is refused
// in the box before the request. Keyed by the wire property, which is how
// backend/gates/settingbounds_test.go holds each to the contract. The two
// sweeps that may be switched off start at 0, and their rows refuse 1 to 299
// as the server does.
const OPERATION_BOUNDS = {
  agent_runner_interval_seconds: { min: 10, max: 3_600 },
  webhook_retry_interval_seconds: { min: 10, max: 3_600 },
  time_scan_interval_seconds: { min: 60, max: 86_400 },
  close_date_sweep_interval_seconds: { min: 3_600, max: 604_800 },
  follow_up_reconcile_interval_seconds: { min: 3_600, max: 604_800 },
  retention_sweep_interval_seconds: { min: 3_600, max: 604_800 },
  geocode_backfill_interval_seconds: { min: 0, max: 604_800 },
  technical_backfill_interval_seconds: { min: 0, max: 604_800 },
  gmail_watch_scan_interval_seconds: { min: 600, max: 43_200 },
  graph_watch_scan_interval_seconds: { min: 600, max: 43_200 },
  gmail_watch_renew_within_hours: { min: 24, max: 144 },
  graph_watch_renew_within_hours: { min: 24, max: 60 },
  send_rate_limit: { min: 1, max: 1_000 },
  send_rate_window_seconds: { min: 10, max: 3_600 },
  send_max_age_hours: { min: 1, max: 168 },
} as const satisfies Record<keyof Operations, { min: number; max: number }>;

type OperationRow = Readonly<{
  property: keyof Operations;
  // The lowest value that switches on a sweep whose 0 means off.
  lowestOn?: number;
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
  {
    property: "geocode_backfill_interval_seconds",
    copy: "geocode",
    lowestOn: 300,
  },
  {
    property: "technical_backfill_interval_seconds",
    copy: "technical",
    lowestOn: 300,
  },
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

type Unit = "seconds" | "hours";

// The wire property names its unit; a count such as the send limit has none.
function unitOf(row: OperationRow): Unit | undefined {
  if (row.property.endsWith("_hours")) return "hours";
  if (row.property.endsWith("_seconds")) return "seconds";
  return undefined;
}

const UNIT_SECONDS: Record<Unit, number> = { seconds: 1, hours: 3_600 };

// The box holds the wire's unit. The line under it names that unit and, where
// it differs, the span in words ("86,400 seconds = 1 day").
function unitReading(
  unit: Unit,
  typed: number,
  { t, plural, locale }: Words,
): string {
  const count = formatNumber(typed, locale);
  const amount =
    unit === "hours"
      ? plural("operations.unit.hours", typed, { count })
      : plural("operations.unit.seconds", typed, { count });
  const interval = formatInterval(typed * UNIT_SECONDS[unit], locale);
  return amount === interval
    ? interval
    : t("operations.reading", { amount, interval });
}

function inWords(
  row: OperationRow,
  words: Words,
): ((typed: number) => string) | undefined {
  const unit = unitOf(row);
  if (unit === undefined) {
    return undefined;
  }
  return (typed) =>
    typed === 0 && row.lowestOn !== undefined
      ? words.t("operations.off")
      : unitReading(unit, typed, words);
}

// The range is said only once a typed value breaks it, in the row's own unit.
function refusalOf(row: OperationRow, { t, locale }: Words): string {
  const bounds = OPERATION_BOUNDS[row.property];
  const range = boundsText(
    { min: row.lowestOn ?? bounds.min, max: bounds.max },
    locale,
  );
  const unit = unitOf(row);
  if (unit === "hours") {
    return t("operations.refusal.hours", range);
  }
  if (unit === undefined) {
    return t("operations.refusal.count", range);
  }
  return row.lowestOn === undefined
    ? t("operations.refusal.seconds", range)
    : t("operations.refusal.secondsOrOff", range);
}

// One card over a slice of the installation's operating values. The page opens
// on `job_health:read`, which a custom role can hold without
// `installation_settings:read`; a card narrower than its page withholds itself
// rather than drawing a refusal the reader can do nothing about.
function OperationsCard({
  title,
  sub,
  rows,
  fold,
}: Readonly<{
  title: string;
  sub: string;
  rows: readonly OperationRow[];
  /** Folds the rows behind this summary, for values seldom changed. */
  fold?: string;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const words = { t, plural, locale };
  const canRead = useCan("installation_settings", "read");
  const canManage = useCanWrite("installation_settings", "update");
  const visible = useHealthVisible();
  const query = useInstallationSettings(canRead && visible);
  const [params] = useUrlParams();
  const section = params.get("section");
  const requested = rows.find(
    (row) => `operations.${row.copy}.label` === section,
  );
  const focused = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (!requested) focused.current = undefined;
    if (!visible || !query.data || !requested || focused.current === section)
      return;
    const control = document.querySelector<HTMLInputElement>(
      `input[data-testid="operation-${requested.property}"]`,
    );
    if (control) {
      control.focus();
      control.scrollIntoView?.({ block: "center" });
      focused.current = section;
    }
  }, [query.data, requested, section, visible]);
  const update = useUpdateInstallationSettings(() => undefined);
  // In a page the bar shows a refused save, so the card would say it twice.
  const inPage = useSavePage() !== null;
  if (!canRead) {
    return null;
  }
  const listOf = ({ operations, locks }: InstallationSettings) => (
    <SettingList bleed={fold ? undefined : "settings"}>
      {rows.map((row) => (
        <NumberSettingRow
          key={row.property}
          label={t(`operations.${row.copy}.label`)}
          description={t(`operations.${row.copy}.help`)}
          testId={`operation-${row.property}`}
          value={operations[row.property]}
          {...OPERATION_BOUNDS[row.property]}
          lowestOn={row.lowestOn}
          refusal={refusalOf(row, words)}
          reading={inWords(row, words)}
          disabled={!canManage || update.isPending}
          reason={lockReasonOr(
            locks,
            `installation.${row.property}`,
            t,
            undefined,
          )}
          onCommit={(next) =>
            inPage
              ? update.mutateAsync({ [row.property]: next })
              : update.mutate({ [row.property]: next })
          }
        />
      ))}
    </SettingList>
  );
  // The gate stands outside the fold, so a failed read shows while it is shut.
  const gated = (
    <QueryGate query={query} pendingLabel={title}>
      {(settings) =>
        fold ? (
          <Disclosure summary={fold} open={requested ? true : undefined}>
            {listOf(settings)}
          </Disclosure>
        ) : (
          listOf(settings)
        )
      }
    </QueryGate>
  );
  return (
    <Panel title={title}>
      <PanelBody className="form-stack">
        <PanelIntro>{sub}</PanelIntro>
        {!canManage && <PanelIntro>{t("operations.adminOnly")}</PanelIntro>}
        {fold && gated}
      </PanelBody>
      {!fold && gated}
      <PanelNotices
        refused={
          update.isError && !inPage
            ? { title: t("operations.updateFailed"), error: update.error }
            : undefined
        }
      />
    </Panel>
  );
}

// How often each background pass runs, and how far ahead a mail subscription
// is renewed. Folded, because an operator sets these once and reads them rarely.
export function BackgroundSchedulesCard() {
  const t = useT();
  return (
    <OperationsCard
      title={t("operations.schedules.title")}
      sub={t("operations.schedules.sub")}
      rows={SCHEDULE_ROWS}
      fold={t("operations.schedules.fold")}
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
