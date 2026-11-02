// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import type { components } from "../../api/schema";
import { ActionRow } from "../../design-system/actionrow";
import { Button } from "../../design-system/atoms";
import { useClipboardCopy } from "../../design-system/clipboardcopy";
import { DataTable } from "../../design-system/datatable";
import { FilterPills } from "../../design-system/filterpills";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import {
  formatDateTime,
  formatNumber,
  identifierNumber,
} from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCan } from "../../lib/capability";
import { useUrlParams } from "../../lib/urlstate";
import { AutomaticWorkHealthCard } from "./automaticworkhealth";
import { CaptureHealthCard } from "./capturehealth";
import { DataTasksHealthCard } from "./datataskshealth";
import { EmbedReindexCard } from "./embedreindex";
import { ExtensionIngestHealthCard } from "./extingesthealth";
import { JobHealthCard } from "./jobhealth";
import { ProviderHealthCard } from "./providerhealth";
import { PublicOriginCard } from "./public-origin";
import { RecoveryHealthCard } from "./recoveryhealth";
import { healthSourceLabels } from "./systemhealth.copy";
import { useJobHistory } from "./systemhealth.queries";
import { jobRunEvidence } from "./systemhealth.report";
import { FindingPanel } from "./systemhealthfinding";
import { healthSourceForSection } from "./systemhealthnavigation";

type Report = components["schemas"]["SystemHealth"];
type State = "retryable" | "discarded" | "cancelled";
const jobStateLabels: Readonly<Record<State, MessageKey>> = {
  retryable: "jobs.state.retryable",
  discarded: "jobs.state.discarded",
  cancelled: "jobs.state.cancelled",
};

export default function SystemHealthDiagnostics({
  report,
}: Readonly<{ report: Report }>) {
  const t = useT();
  const [params] = useUrlParams();
  const [selected, setSource] = useState<string>();
  const requested = selected ?? healthSourceForSection(params.get("section"));
  const allowed = report.sources.filter((s) => s.availability !== "withheld");
  const source = allowed.some((s) => s.id === requested)
    ? requested
    : (allowed[0]?.id ?? "none");
  return (
    <>
      <FilterPills
        label={t("health.capabilities")}
        value={source ?? "none"}
        onChange={setSource}
        pills={report.sources
          .filter((s) => s.availability !== "withheld")
          .map((s) => ({
            value: s.id,
            label: t(healthSourceLabels[s.id] ?? "health.unknown"),
          }))}
      />
      {report.findings
        .filter((f) => f.source === source && f.requires_action)
        .map((f) => (
          <FindingPanel key={f.id} finding={f} detail />
        ))}
      {source === "jobs" && (
        <>
          <JobHistory />
          <JobHealthCard showHistory={false} />
        </>
      )}
      {source === "capture" && <CaptureHealthCard />}
      {source === "intake" && <ExtensionIngestHealthCard />}
      {source === "recovery" && <RecoveryHealthCard />}
      {source === "automatic" && <AutomaticWorkHealthCard />}
      {source === "providers" && <ProviderHealthCard />}
      {source === "search" && <EmbedReindexCard />}
      {source === "updates" && <DataTasksHealthCard />}
      <PublicOriginCard status={report.public_origin} />
    </>
  );
}

function JobHistory() {
  const canSee = useCan("job_health", "read");
  return canSee ? <JobHistoryBody /> : null;
}

function JobHistoryBody() {
  const t = useT();
  const { locale } = useLocale();
  const [params] = useUrlParams();
  const requestedState = params.get("state");
  const [state, setState] = useState<State | undefined>(
    requestedState === "discarded" ||
      requestedState === "retryable" ||
      requestedState === "cancelled"
      ? requestedState
      : undefined,
  );
  const [days, setDays] = useState(7);
  const [kind, setKind] = useState<string | undefined>(
    params.get("kind") || undefined,
  );
  const [cursor, setCursor] = useState<string | undefined>();
  const query = useJobHistory({ state, days, kind }, cursor);
  const changeState = (value: string) => {
    setState(
      value === "retryable" || value === "discarded" || value === "cancelled"
        ? value
        : undefined,
    );
    setCursor(undefined);
  };
  return (
    <Panel title={t("health.history")}>
      <PanelBody>
        <QueryGate query={query} pendingLabel={t("health.history")}>
          {(report) => (
            <>
              {report.runs[0] && (
                <>
                  <p className="t-caption">
                    {t("health.recordedRun", {
                      id:
                        report.runs[0].job_id === undefined
                          ? t("health.unknown")
                          : identifierNumber(report.runs[0].job_id),
                      time: formatDateTime(
                        report.runs[0].failed_at,
                        locale,
                        viewerZone(),
                      ),
                      state: t(jobStateLabels[report.runs[0].state]),
                    })}
                  </p>
                  <p>
                    <strong>{report.runs[0].reason}</strong>{" "}
                    {report.runs[0].remedy}
                  </p>
                </>
              )}

              <p>{t("health.historyCoverage")}</p>
              <p>{t("health.historyWindow")}</p>
              <FilterPills
                value={state ?? "all"}
                onChange={changeState}
                label={t("jobs.col.state")}
                pills={[
                  { value: "all", label: t("health.all") },
                  { value: "retryable", label: t("jobs.state.retryable") },
                  { value: "discarded", label: t("jobs.state.discarded") },
                  { value: "cancelled", label: t("jobs.state.cancelled") },
                ]}
              />
              <FilterPills
                value={days === 1 ? "1" : "7"}
                onChange={(value) => {
                  setDays(Number(value));
                  setCursor(undefined);
                }}
                label={t("health.window")}
                pills={[
                  { value: "1", label: t("health.oneDay") },
                  { value: "7", label: t("health.sevenDays") },
                ]}
              />
              <p className="t-caption">
                {t("systemHealth.asOf", {
                  time: formatDateTime(
                    report.observed_at,
                    locale,
                    viewerZone(),
                  ),
                })}
              </p>
              {report.population_may_have_changed && (
                <p role="status">{t("health.historyChanged")}</p>
              )}
              <DataTable
                label={t("health.groups")}
                rows={report.groups}
                rowKey={(g) => JSON.stringify([g.kind, g.state, g.fleet_wide])}
                columns={[
                  {
                    key: "kind",
                    header: t("jobs.col.job"),
                    render: (g) => g.kind,
                  },
                  {
                    key: "scope",
                    header: t("health.scope"),
                    render: (g) =>
                      t(
                        g.fleet_wide
                          ? "health.sharedScope"
                          : "health.workspaceScope",
                      ),
                  },
                  {
                    key: "state",
                    header: t("jobs.col.state"),
                    render: (g) => t(jobStateLabels[g.state]),
                  },
                  {
                    key: "count",
                    header: t("health.unit.runs"),
                    render: (g) => formatNumber(g.count, locale),
                  },
                  {
                    key: "first",
                    header: t("health.firstObserved"),
                    render: (group) =>
                      formatDateTime(group.first_at, locale, viewerZone()),
                  },
                  {
                    key: "open",
                    header: t("health.inspect"),
                    render: (g) => (
                      <Button
                        onClick={() => {
                          setKind(g.kind);
                          setCursor(undefined);
                        }}
                      >
                        {t("health.inspect")}
                      </Button>
                    ),
                  },
                ]}
              />
              {kind && (
                <Button
                  onClick={() => {
                    setKind(undefined);
                    setCursor(undefined);
                  }}
                >
                  {t("health.all")}
                </Button>
              )}
              <DataTable
                label={t("jobs.failures")}
                fold
                rows={report.runs}
                rowKey={(run) => String(run.job_id)}
                columns={[
                  {
                    key: "id",
                    header: t("jobs.col.jobId"),
                    render: (run) =>
                      run.job_id === undefined
                        ? null
                        : identifierNumber(run.job_id),
                  },
                  {
                    key: "kind",
                    header: t("jobs.col.job"),
                    render: (run) => run.kind,
                  },
                  {
                    key: "reason",
                    header: t("jobs.col.reason"),
                    render: (run) => run.reason,
                  },
                  {
                    key: "attempt",
                    header: t("jobs.col.attempt"),
                    render: (run) =>
                      `${formatNumber(run.attempt, locale)} / ${formatNumber(run.max_attempts, locale)}`,
                  },
                  {
                    key: "copy",
                    header: t("health.copy"),
                    render: (run) => (
                      <JobRunCopy run={run} observedAt={report.observed_at} />
                    ),
                  },
                  {
                    key: "at",
                    header: t("jobs.col.failedAt"),
                    render: (run) =>
                      formatDateTime(run.failed_at, locale, viewerZone()),
                  },
                ]}
              />
              <ActionRow>
                <Button
                  onClick={() => {
                    if (cursor === undefined)
                      return query.refetch({ cancelRefetch: false });
                    setCursor(undefined);
                  }}
                >
                  {t("health.refresh")}
                </Button>
                {report.has_more && (
                  <Button onClick={() => setCursor(report.next_cursor)}>
                    {t("health.nextPage")}
                  </Button>
                )}
              </ActionRow>
            </>
          )}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}

function JobRunCopy({
  run,
  observedAt,
}: Readonly<{ run: components["schemas"]["JobFailure"]; observedAt: string }>) {
  const t = useT();
  const text = JSON.stringify(
    { observed_at: observedAt, ...jobRunEvidence(run) },
    null,
    2,
  );
  const copy = useClipboardCopy(text, {
    copy: t("health.copy"),
    copied: t("health.copied"),
    remedy: t("health.copyRemedy"),
  });
  return (
    <>
      <Button onClick={copy.copy}>{copy.label}</Button>
      {copy.notice}
    </>
  );
}
