// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { EmptyState } from "../../design-system/atoms";
import { type Fact, FactList } from "../../design-system/factlist";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { StatusBadge, type StatusKind } from "../../design-system/statusbadge";
import { formatDateTime, formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCan } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import { HealthCard } from "./healthcard";

// GET /admin/recovery-health: when the restore procedure was last rehearsed,
// and what it measured against the published targets. The drill ledger is
// written by the operator's margince-migrate drill verbs, never from here.

type Health = components["schemas"]["RecoveryHealth"];
type Drill = components["schemas"]["RestoreDrill"];

const OUTCOME: Record<
  Drill["outcome"],
  { label: MessageKey; kind: StatusKind; live?: boolean }
> = {
  passed: { label: "recoveryHealth.outcome.passed", kind: "done" },
  failed: { label: "recoveryHealth.outcome.failed", kind: "failure" },
  running: {
    label: "recoveryHealth.outcome.running",
    kind: "neutral",
    live: true,
  },
};

// Hours and minutes: a recovery window is read against a four-hour target, and
// the whole-hour floor of formatDuration would hide most of that range.
function formatWindow(seconds: number, t: Translator, locale: Locale): string {
  const minutes = Math.floor(seconds / 60);
  const hours = formatNumber(Math.floor(minutes / 60), locale);
  if (minutes % 60 === 0) {
    return t("recoveryHealth.windowHours", { hours });
  }
  return t("recoveryHealth.window", {
    hours,
    minutes: formatNumber(minutes % 60, locale),
  });
}

function againstTarget(
  seconds: number,
  target: number,
  t: Translator,
  locale: Locale,
) {
  const key: MessageKey =
    seconds <= target
      ? "recoveryHealth.withinTarget"
      : "recoveryHealth.overTarget";
  return t(key, { target: formatWindow(target, t, locale) });
}

function drillFacts(
  health: Health,
  drill: Drill,
  t: Translator,
  locale: Locale,
  zone: string,
): Fact[] {
  const outcome = OUTCOME[drill.outcome];
  const facts: Fact[] = [
    {
      key: "outcome",
      term: t("recoveryHealth.outcome"),
      value: (
        <StatusBadge
          kind={outcome.kind}
          label={t(outcome.label)}
          live={outcome.live}
        />
      ),
      note: t("recoveryHealth.startedAt", {
        when: formatDateTime(drill.started_at, locale, zone),
      }),
    },
    {
      key: "recovery",
      term: t("recoveryHealth.recovery"),
      value:
        drill.recovery_seconds === null
          ? t("recoveryHealth.notFinished")
          : formatWindow(drill.recovery_seconds, t, locale),
      note:
        drill.recovery_seconds === null
          ? undefined
          : againstTarget(
              drill.recovery_seconds,
              health.recovery_target_seconds,
              t,
              locale,
            ),
    },
    {
      key: "dataLoss",
      term: t("recoveryHealth.dataLoss"),
      value: formatWindow(drill.data_loss_seconds, t, locale),
      note: againstTarget(
        drill.data_loss_seconds,
        health.data_loss_target_seconds,
        t,
        locale,
      ),
    },
    {
      key: "restoredTo",
      term: t("recoveryHealth.restoredTo"),
      value: formatDateTime(drill.restored_to, locale, zone),
    },
    {
      key: "operator",
      term: t("recoveryHealth.operator"),
      value: drill.operator,
    },
  ];
  if (drill.notes) {
    facts.push({
      key: "notes",
      term: t("recoveryHealth.notes"),
      value: drill.notes,
    });
  }
  return facts;
}

const isSeconds = (value: unknown): value is number =>
  typeof value === "number" && Number.isFinite(value) && value >= 0;
const isText = (value: unknown): value is string => typeof value === "string";
const orNull =
  (check: (value: unknown) => boolean) =>
  (value: unknown): boolean =>
    value === null || check(value);

// The payload is typed by the contract, but the wire is not: every field the
// card draws is checked again at runtime.
function isDrill(drill: Drill): boolean {
  return (
    isText(drill.started_at) &&
    orNull(isText)(drill.finished_at) &&
    isText(drill.restored_to) &&
    Object.hasOwn(OUTCOME, drill.outcome) &&
    isText(drill.operator) &&
    orNull(isText)(drill.notes) &&
    orNull(isSeconds)(drill.recovery_seconds) &&
    isSeconds(drill.data_loss_seconds)
  );
}

// A missing number renders as "NaN h" and a missing last_drill would read as
// "never drilled". Only an explicit null says that; anything else malformed is
// the card's error state.
function isRecoveryHealth(health: Health | undefined): health is Health {
  return (
    typeof health === "object" &&
    health !== null &&
    isText(health.generated_at) &&
    isSeconds(health.recovery_target_seconds) &&
    isSeconds(health.data_loss_target_seconds) &&
    (health.last_drill === null ||
      (typeof health.last_drill === "object" && isDrill(health.last_drill)))
  );
}

// A drill's facts stand in their own row. No drill at all is one line in the
// intro, said outright: it is when the targets are least proven.
function RecoveryBody({
  health,
  zone,
}: Readonly<{ health: Health; zone: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const drill = health.last_drill;

  return (
    <SettingList bleed="settings">
      {drill && (
        <SettingRow
          label={t("recoveryHealth.lastDrill")}
          description={t("recoveryHealth.lastDrillHint")}
          layout="stack"
          control={
            <div className="settingrow-measure">
              <FactList facts={drillFacts(health, drill, t, locale, zone)} />
            </div>
          }
        />
      )}
      <SettingRow
        label={t("recoveryHealth.lastBackup")}
        description={t("recoveryHealth.lastBackupHint")}
        control={<span>{t("recoveryHealth.backupNotObserved")}</span>}
      />
    </SettingList>
  );
}

// `job_health:read`, what the endpoint asks for (compose/recoveryhealth.go),
// the grant every System health card shares.
export function useRecoveryHealth() {
  const canSee = useCan("job_health", "read");
  const query = useQuery({
    queryKey: ["recovery-health"],
    enabled: canSee,
    queryFn: async () => {
      const data = unwrap(await api.GET("/admin/recovery-health"));
      if (!isRecoveryHealth(data)) {
        throw new Error("malformed recovery-health response");
      }
      return data;
    },
  });
  return { canSee, query };
}

export function RecoveryHealthCard() {
  const t = useT();
  const zone = viewerZone();
  const { canSee, query } = useRecoveryHealth();

  return (
    <HealthCard
      title={t("settings.recoveryHealth")}
      sub={t("settings.recoveryHealthSub")}
      withheld={t("recoveryHealth.adminOnly")}
      canSee={canSee}
      query={query}
      lead={(health) =>
        health.last_drill === null && (
          <EmptyState>{t("recoveryHealth.never")}</EmptyState>
        )
      }
    >
      {(health) => <RecoveryBody health={health} zone={zone} />}
    </HealthCard>
  );
}
