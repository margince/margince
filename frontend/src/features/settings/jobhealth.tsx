// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Badge, EmptyState } from "../../design-system/atoms";
import { CellStack } from "../../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { KeyedName } from "../../design-system/keyedname";
import {
  PanelBody,
  PanelGroupHead,
  PanelIntro,
} from "../../design-system/panel";
import {
  formatDateTime,
  formatNumber,
  identifierNumber,
  ordinalNumber,
} from "../../format/format";
import { formatAge, formatInterval } from "../../format/interval";
import { viewerZone } from "../../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCan } from "../../lib/capability";
import { useHealthScope, useHealthVisible } from "../../lib/healthscope";
import { unwrap } from "../../lib/problem";
import { HealthCard } from "./healthcard";
import { jobKindNameKey } from "./jobkinds";
import { healthConditions } from "./systemhealth.copy";

// GET /admin/job-health — the operator's only window onto the background
// system, for Settings → Maintenance. Until this card existed a stalled queue
// had no screen at all: the endpoint shipped and nothing read it.
//
// The read is the admin's, gated server-side on the ROLE and on a human
// session (x-agent-access: human-only) rather than on any RBAC object — none
// describes background work, and the report spans the whole installation, not
// just the reader's own records.

type JobHealth = components["schemas"]["JobHealth"];
type JobKindHealth = components["schemas"]["JobKindHealth"];
type JobFailure = components["schemas"]["JobFailure"];

// The closed failure-state enum with the tone each state earns. `cancelled`
// earns none: it was stopped deliberately, so it records a decision somebody
// made rather than a fault the reader has to chase. Keying the map on the union
// makes a state added upstream a compile error here instead of an untoned badge.
const FAILURE_STATE: Record<
  JobFailure["state"],
  Readonly<{ label: MessageKey; tone: "warning" | "danger" | undefined }>
> = {
  retryable: { label: "jobs.state.retryable", tone: "warning" },
  discarded: { label: "jobs.state.discarded", tone: "danger" },
  cancelled: { label: "jobs.state.cancelled", tone: undefined },
};

// River's kind stays under the name: it is the string the worker log carries.
function JobKind({ kind }: Readonly<{ kind: string }>) {
  const t = useT();
  const name = jobKindNameKey(kind);
  return <KeyedName name={name === undefined ? kind : t(name)} code={kind} />;
}

const COUNTED = [
  "waiting",
  "running",
  "retrying",
  "discarded",
  "cancelled",
] as const;
type Counted = (typeof COUNTED)[number];

const COUNT_HEADER: Record<Counted, MessageKey> = {
  waiting: "jobs.col.waiting",
  running: "jobs.col.running",
  retrying: "jobs.col.retrying",
  discarded: "jobs.state.discarded",
  cancelled: "jobs.state.cancelled",
};

// A retry or a dead job above zero is the figure an operator came to find.
const COUNT_TONE: Partial<Record<Counted, "warning" | "danger">> = {
  retrying: "warning",
  discarded: "danger",
};

// Every count, zeros included: "0 dead" is the reassurance an operator came for.
function kindColumns(
  t: Translator,
  locale: Locale,
): DataTableColumn<JobKindHealth>[] {
  const counts = COUNTED.map(
    (count): DataTableColumn<JobKindHealth> => ({
      key: count,
      header: t(COUNT_HEADER[count]),
      align: "end",
      render: (kind) => {
        const tone = COUNT_TONE[count];
        const value = kind[count];
        if (value === undefined) return t("health.unknown");
        const shown = formatNumber(value, locale);
        return tone && value > 0 ? <Badge tone={tone}>{shown}</Badge> : shown;
      },
    }),
  );
  return [
    {
      key: "kind",
      header: t("jobs.col.job"),
      render: (kind) => <JobKind kind={kind.kind} />,
    },
    {
      key: "queue",
      header: t("jobs.col.queue"),
      render: (kind) => <code>{kind.queue}</code>,
    },
    {
      key: "condition",
      header: t("jobs.col.state"),
      render: (kind) =>
        t(healthConditions[kind.condition ?? "unknown"] ?? "health.unknown"),
    },
    ...counts,
    {
      key: "expectation",
      header: t("jobs.col.expectation"),
      render: (kind) =>
        kind.expectation_seconds === undefined
          ? t("health.unknown")
          : formatInterval(kind.expectation_seconds, locale),
    },
    {
      key: "oldest",
      header: t("jobs.col.oldestWait"),
      align: "end",
      // A null age means nothing is runnable now, which is not a wait of zero.
      render: (kind) =>
        kind.oldest_waiting_age_seconds === null
          ? null
          : formatAge(kind.oldest_waiting_age_seconds, locale),
    },
  ];
}

function KindSection({
  label,
  description,
  kinds,
  emptyText,
}: Readonly<{
  label: string;
  description?: string;
  kinds: readonly JobKindHealth[];
  emptyText: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <>
      <PanelGroupHead title={label} level="h3" />
      {(description || kinds.length === 0) && (
        <PanelBody>
          {description && <PanelIntro>{description}</PanelIntro>}
          {kinds.length === 0 && <EmptyState>{emptyText}</EmptyState>}
        </PanelBody>
      )}
      {kinds.length > 0 && (
        <DataTable
          bleed
          stickyFirst
          label={label}
          columns={kindColumns(t, locale)}
          rows={[...kinds]}
          // A kind is unique per queue only, so the pair names the row.
          rowKey={(kind) => JSON.stringify([kind.queue, kind.kind])}
        />
      )}
    </>
  );
}

function failureColumns(
  t: Translator,
  locale: Locale,
  zone: string,
): DataTableColumn<JobFailure>[] {
  return [
    {
      key: "kind",
      header: t("jobs.col.job"),
      render: (failure) => <JobKind kind={failure.kind} />,
    },
    {
      key: "state",
      header: t("jobs.col.state"),
      fold: "end",
      render: (failure) => {
        const state = FAILURE_STATE[failure.state];
        return <Badge tone={state.tone}>{t(state.label)}</Badge>;
      },
    },
    {
      key: "reason",
      header: t("jobs.col.reason"),
      // The job layer's vetted sentence, as it arrived; the class beside it is
      // the token an operator keys an alert on, so it stays verbatim.
      render: (failure) => (
        <CellStack>
          <span>{failure.reason}</span>
          {failure.remedy && (
            <span className="t-caption">
              {t("jobs.remedy", { remedy: failure.remedy })}
            </span>
          )}
          {failure.failure_class && (
            <code className="t-caption">{failure.failure_class}</code>
          )}
        </CellStack>
      ),
    },
    {
      key: "attempt",
      header: t("jobs.col.attempt"),
      align: "end",
      render: (failure) =>
        t("jobs.attemptOf", {
          attempt: ordinalNumber(failure.attempt),
          max: ordinalNumber(failure.max_attempts),
        }),
      folded: (failure) =>
        t("jobs.attempt", {
          attempt: ordinalNumber(failure.attempt),
          max: ordinalNumber(failure.max_attempts),
        }),
    },
    {
      key: "failedAt",
      header: t("jobs.col.failedAt"),
      // A first failure only when an attempt error was recorded: a job cancelled
      // before it ran has none, and failed_at would fake a span.
      render: (failure) => (
        <CellStack>
          <span>{formatDateTime(failure.failed_at, locale, zone)}</span>
          {failure.first_failed_at && (
            <span className="t-caption">
              {t("jobs.failingSince", {
                when: formatDateTime(failure.first_failed_at, locale, zone),
              })}
            </span>
          )}
        </CellStack>
      ),
    },
    {
      key: "jobId",
      header: t("jobs.col.jobId"),
      align: "end",
      fold: "hide",
      render: (failure) =>
        failure.job_id === undefined ? null : identifierNumber(failure.job_id),
    },
  ];
}

function FailureSection({
  failures,
  zone,
}: Readonly<{ failures: readonly JobFailure[]; zone: string }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <>
      <PanelGroupHead title={t("jobs.failures")} level="h3" />
      <PanelBody>
        <PanelIntro>{t("jobs.failuresSub")}</PanelIntro>
        {failures.length === 0 && (
          <EmptyState>{t("jobs.failuresEmpty")}</EmptyState>
        )}
      </PanelBody>
      {failures.length > 0 && (
        <>
          <DataTable
            bleed
            fold
            label={t("jobs.failures")}
            columns={failureColumns(t, locale, zone)}
            rows={[...failures]}
            // A server-ordered snapshot: two failures of one kind can share a
            // timestamp, so position is what tells them apart.
            rowKey={(failure) =>
              JSON.stringify([failures.indexOf(failure), failure.kind])
            }
          />
          <PanelBody>
            <p className="t-caption">{t("jobs.reasonVetted")}</p>
          </PanelBody>
        </>
      )}
    </>
  );
}

function JobHealthBody({
  health,
  zone,
  showHistory,
}: Readonly<{ health: JobHealth; zone: string; showHistory: boolean }>) {
  const t = useT();

  if (health.kinds.length === 0 && health.recent_failures.length === 0) {
    // Its own words rather than the generic "nothing here": that the background
    // system is idle is a finding, and the reader must be able to tell it from
    // a card that had nothing to show them.
    return (
      <PanelBody>
        <EmptyState>{t("jobs.empty")}</EmptyState>
      </PanelBody>
    );
  }

  return (
    <>
      <KindSection
        label={t("jobs.workspaceKinds")}
        kinds={health.kinds.filter((kind) => !kind.fleet_wide)}
        emptyText={t("jobs.workspaceEmpty")}
      />
      <KindSection
        label={t("jobs.dispatcherKinds")}
        description={t("jobs.dispatcherSub")}
        kinds={health.kinds.filter((kind) => kind.fleet_wide)}
        emptyText={t("jobs.dispatcherEmpty")}
      />
      {showHistory && (
        <FailureSection failures={health.recent_failures} zone={zone} />
      )}
    </>
  );
}

// `job_health:read`, what the endpoint asks for (compose/jobhealth.go); without
// it the call could only answer 403, so it is not made.
export function useJobHealth() {
  const canSee = useCan("job_health", "read");
  const scope = useHealthScope();
  const visible = useHealthVisible();
  const query = useQuery({
    queryKey: ["job-health", scope],
    enabled: canSee && visible && scope !== undefined,
    queryFn: async ({ signal }) => {
      const data = unwrap(await api.GET("/admin/job-health", { signal }));
      // Rejected, not defaulted: `?? []` would draw the idle state, a claim about
      // the installation this response never made.
      if (
        !data ||
        typeof data.generated_at !== "string" ||
        !Array.isArray(data.kinds) ||
        !Array.isArray(data.recent_failures)
      ) {
        throw new Error("malformed job-health response");
      }
      return data;
    },
  });
  return { canSee, query };
}

export function JobHealthCard({
  showHistory = true,
}: Readonly<{ showHistory?: boolean }>) {
  const t = useT();
  // The reader's own zone: a stalled-queue time is only useful against the
  // clock on their wall.
  const zone = viewerZone();
  const { canSee, query } = useJobHealth();

  return (
    <HealthCard
      title={t("settings.jobs")}
      sub={t("settings.jobsSub")}
      withheld={t("jobs.adminOnly")}
      canSee={canSee}
      query={query}
    >
      {(health) => (
        <JobHealthBody health={health} zone={zone} showHistory={showHistory} />
      )}
    </HealthCard>
  );
}
