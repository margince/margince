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
import { useLocale, useT } from "../../i18n";
import { useCan } from "../../lib/capability";
import { useHealthScope } from "../../lib/healthscope";
import { navigate } from "../../lib/router";
import { settingsHref } from "../../lib/settingsrouting";
import { useUrlParams } from "../../lib/urlstate";
import {
  healthAvailability,
  healthCodes,
  healthConditions,
  healthSourceLabels,
} from "./systemhealth.copy";
import { useSystemHealth } from "./systemhealth.queries";
import { FindingPanel } from "./systemhealthfinding";

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
                    key={`${scope}:${params.get("section") ?? ""}:${params.get("finding") ?? ""}`}
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
    report.findings.filter((f) => f.requires_action).map((f) => f.id),
  );
  const actionable = report.findings.filter((f) => f.requires_action);
  const changed = actionable.some((f, i) => order[i] !== f.id);
  const shown = [...actionable].sort((a, b) => {
    const ai = order.indexOf(a.id),
      bi = order.indexOf(b.id);
    return (ai < 0 ? order.length : ai) - (bi < 0 ? order.length : bi);
  });
  const title = stale
    ? t("health.incomplete")
    : actionable.length
      ? t("health.attention")
      : t("health.checked");
  return (
    <>
      <ActionRow>
        <Heading size="small" as="h2">
          {title}
        </Heading>
        <Button
          pending={refreshing}
          onClick={async () => {
            const refreshed = await refresh();
            if (refreshed)
              setOrder(
                refreshed.findings
                  .filter((f) => f.requires_action)
                  .map((f) => f.id),
              );
          }}
        >
          {t("health.refresh")}
        </Button>
      </ActionRow>
      {changed && <p role="status">{t("health.prioritiesChanged")}</p>}
      {shown.map((finding) => (
        <FindingPanel key={finding.id} finding={finding} />
      ))}
      <Disclosure summary={t("health.capabilities")}>
        {report.sources
          .filter((source) => source.availability !== "withheld")
          .map((source) => (
            <SourceReading key={source.id} source={source} />
          ))}
      </Disclosure>
    </>
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
  const available = source.availability === "available";
  const tone =
    source.availability === "failed" ||
    source.condition === "technical" ||
    source.code === "queue_unverified"
      ? "danger"
      : ["retrying", "waiting_human", "unverified"].includes(source.condition)
        ? "warning"
        : source.condition === "processing" ||
            (available && ["clear", "recovered"].includes(source.condition))
          ? "success"
          : "default";
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
