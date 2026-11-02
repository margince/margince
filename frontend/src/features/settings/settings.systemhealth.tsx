// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { lazy, Suspense, useState } from "react";
import type { components } from "../../api/schema";
import { ActionRow } from "../../design-system/actionrow";
import { Badge, Button } from "../../design-system/atoms";
import { Disclosure } from "../../design-system/disclosure";
import { FactList } from "../../design-system/factlist";
import { FilterPills } from "../../design-system/filterpills";
import { Heading } from "../../design-system/heading";
import { QueryGate } from "../../design-system/querystates";
import { formatDateTime, formatNumber } from "../../format/format";
import { formatAge } from "../../format/interval";
import { viewerZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import { useCan } from "../../lib/capability";
import { useHealthScope } from "../../lib/healthscope";
import { navigate } from "../../lib/router";
import { settingsHref } from "../../lib/settingsrouting";
import { useUrlParams } from "../../lib/urlstate";
import { PublicOriginCard } from "./public-origin";
import {
  healthAvailability,
  healthCodes,
  healthConditions,
  healthSourceLabels,
} from "./systemhealth.copy";
import { findingNextStepKey } from "./systemhealth.effect";
import { useSystemHealth } from "./systemhealth.queries";
import { FindingPanel } from "./systemhealthfinding";
import { HealthReadings } from "./systemhealthreadings";
import { healthTone } from "./systemhealthstatus";

const Diagnostics = lazy(() => import("./systemhealthdiagnostics"));
const Configuration = lazy(() => import("./systemhealthconfiguration"));
type Report = components["schemas"]["SystemHealth"];
type Source = components["schemas"]["HealthSource"];
export type HealthView = "overview" | "diagnostics" | "configuration";

export function SystemHealthPage({
  view = "overview",
}: Readonly<{ view?: string }>) {
  const t = useT();
  const canConfigure = useCan("installation_settings", "read");
  const query = useSystemHealth(view !== "configuration");
  const scope = useHealthScope();
  const [params] = useUrlParams();
  return (
    <>
      <FilterPills
        value={view}
        label={t("health.views")}
        pills={[
          { value: "overview", label: t("health.overview") },
          { value: "diagnostics", label: t("health.diagnostics") },
          ...(canConfigure
            ? [{ value: "configuration", label: t("health.configuration") }]
            : []),
        ]}
        onChange={(next) =>
          navigate({
            ...settingsHref("system-health"),
            id2: next === "overview" ? undefined : next,
          })
        }
      />
      {view === "configuration" ? (
        <Suspense fallback={<p role="status">{t("health.loading")}</p>}>
          <Configuration />
        </Suspense>
      ) : (
        <>
          {query.isError && query.data && (
            <p role="status">{t("health.stale")}</p>
          )}
          <QueryGate
            query={{
              ...query,
              isError: query.isError && query.data === undefined,
            }}
            pendingLabel={t("health.overview")}
          >
            {(report) =>
              view === "diagnostics" ? (
                <Suspense fallback={<p role="status">{t("health.loading")}</p>}>
                  <Diagnostics
                    key={`${scope}:${params.get("section") ?? ""}:${params.get("finding") ?? ""}:${params.get("kind") ?? ""}:${params.get("state") ?? ""}`}
                    report={report}
                  />
                </Suspense>
              ) : (
                <Overview
                  key={scope}
                  report={report}
                  stale={query.isError}
                  refresh={async () =>
                    (await query.refetch({ cancelRefetch: false })).data
                  }
                  refreshing={query.isFetching}
                />
              )
            }
          </QueryGate>
        </>
      )}
    </>
  );
}

function Overview({
  report,
  refresh,
  refreshing,
  stale,
}: Readonly<{
  report: Report;
  stale: boolean;
  refresh: () => Promise<Report | undefined>;
  refreshing: boolean;
}>) {
  const t = useT();
  const [order, setOrder] = useState<readonly string[]>(
    actionableFindings(report.findings).map((f) => f.id),
  );
  const actionable = actionableFindings(report.findings);
  const remainingOrder = order.filter((id) =>
    actionable.some((f) => f.id === id),
  );
  const changed = actionable.some((f, i) => remainingOrder[i] !== f.id);
  const shown = [...actionable].sort((a, b) => {
    const ai = order.indexOf(a.id),
      bi = order.indexOf(b.id);
    return (ai < 0 ? order.length : ai) - (bi < 0 ? order.length : bi);
  });
  const title = stale
    ? t("health.refreshNeeded")
    : actionable.length
      ? t("health.attention")
      : report.complete
        ? t("health.checked")
        : t("health.incomplete");
  return (
    <>
      <ActionRow>
        <Heading size="small" as="h2">
          {title}
        </Heading>
        <OverviewStatus findings={actionable} />
        <Button
          pending={refreshing}
          onClick={async () => {
            const refreshed = await refresh();
            if (refreshed)
              setOrder(actionableFindings(refreshed.findings).map((f) => f.id));
          }}
        >
          {t("health.refresh")}
        </Button>
      </ActionRow>
      {changed && <p role="status">{t("health.prioritiesChanged")}</p>}
      <OverviewFindings findings={shown} />
      {!stale && (
        <HealthReadings sources={report.sources} findings={report.findings} />
      )}
      <Disclosure summary={t("health.capabilities")}>
        {report.sources
          .filter((source) => source.availability !== "withheld")
          .map((source) => (
            <SourceReading key={source.id} source={source} />
          ))}
        <PublicOriginCard status={report.public_origin} />
      </Disclosure>
    </>
  );
}

function OverviewFindings({
  findings,
}: Readonly<{ findings: Report["findings"] }>) {
  const groups = new Map<string, Report["findings"]>();
  for (const finding of findings) {
    const key =
      finding.source === "capture" && finding.responsible !== "mailbox_owner"
        ? JSON.stringify([
            finding.source,
            finding.responsible,
            finding.action.destination,
            findingNextStepKey(finding),
          ])
        : finding.id;
    groups.set(key, [...(groups.get(key) ?? []), finding]);
  }
  return Array.from(groups, ([key, entries], index) =>
    entries[0] ? (
      <FindingPanel
        key={key}
        finding={entries[0]}
        lead={index === 0}
        related={entries.length > 1 ? entries : undefined}
      />
    ) : null,
  );
}

// Older servers emit the mailbox total alongside the same sender and thread choices.
function actionableFindings(findings: Report["findings"]) {
  const actionable = findings.filter((finding) => finding.requires_action);
  const specificMailReview = actionable.some((finding) =>
    ["capture_uncertain", "capture_thread_uncertain"].includes(finding.code),
  );
  return actionable.filter(
    (finding) => finding.code !== "capture_owner_review" || !specificMailReview,
  );
}

function OverviewStatus({
  findings,
}: Readonly<{ findings: Report["findings"] }>) {
  const plural = usePlural();
  const { locale } = useLocale();
  if (!findings.length) return null;
  const tone = findings.some(
    (f) => f.requires_action && healthTone(f) === "danger",
  )
    ? "danger"
    : "warning";
  return (
    <Badge tone={tone}>
      {plural("health.actionsNeeded", findings.length, {
        count: formatNumber(findings.length, locale),
      })}
    </Badge>
  );
}

function SourceReading({ source }: Readonly<{ source: Source }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <Disclosure
      summary={
        <>
          {t(healthSourceLabels[source.id] ?? "health.unknown")} ·{" "}
          <SourceStatus source={source} />
        </>
      }
    >
      <p>{t(healthCodes[source.code] ?? "health.unknown")}</p>
      {source.condition !== "unknown" && (
        <p>{t(healthConditions[source.condition] ?? "health.unknown")}</p>
      )}
      {source.observed_at && (
        <p>
          {t("systemHealth.asOf", {
            time: formatDateTime(source.observed_at, locale, viewerZone()),
          })}
        </p>
      )}
      {source.oldest_wait_seconds !== undefined && (
        <p>
          {t("health.oldestWait", {
            age: formatAge(source.oldest_wait_seconds, locale),
          })}
        </p>
      )}

      {source.id === "jobs" && <p>{t("health.queueCoverage")}</p>}
      {source.id === "recovery" && <p>{t("health.backupsUnmonitored")}</p>}
      {source.automatic_work && (
        <FactList
          facts={[
            {
              key: "created",
              term: t("automaticWork.col.created"),
              value: formatNumber(source.automatic_work.created, locale),
            },
            {
              key: "completed",
              term: t("automaticWork.col.completed"),
              value: formatNumber(source.automatic_work.completed, locale),
            },
            {
              key: "open",
              term: t("automaticWork.col.open"),
              value: formatNumber(source.automatic_work.open, locale),
            },
          ]}
        />
      )}
      {source.id === "automatic" && <p>{t("health.automaticLimits")}</p>}
    </Disclosure>
  );
}

function SourceStatus({ source }: Readonly<{ source: Source }>) {
  const t = useT();
  const tone = healthTone(source);
  return (
    <Badge tone={tone}>
      {t(
        healthCodes[source.code] ??
          healthAvailability[source.availability] ??
          "health.failed",
      )}
    </Badge>
  );
}
