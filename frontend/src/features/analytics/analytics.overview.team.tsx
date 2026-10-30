// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type UseQueryResult, useQuery } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { SegmentedControl, StatCard } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { DataTable } from "../../design-system/datatable";
import { FilterBar } from "../../design-system/filterbar";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { StatStrip } from "../../design-system/statstrip";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatDuration, formatHours, formatNumber } from "../../format/format";
import { type Locale, useLocale, usePlural, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { replaceDial, useUrlParams } from "../../lib/urlstate";
import type { AnalyticsScope } from "./analytics.context";
import { QualificationNote } from "./analytics.overview.prospecting";
import {
  MetricReading,
  OverviewLinks,
  OverviewSaveActions,
} from "./analytics.overview.readings";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import {
  type ReportingEvidenceRef,
  type ReportingSelection,
  reportingPeriodLabel,
} from "./reporting.model";
import { resultsThrough } from "./reporting.overview";
import { useReportingEvaluation } from "./reporting.queries";
import { presetRequest } from "./reporting.views";

type Catalog = components["schemas"]["ReportingCatalog"];
type Framework = components["schemas"]["ReportingFramework"];
type Review = components["schemas"]["SdrTeamReview"];
type Handoffs = components["schemas"]["SdrTeamHandoffs"];
type Member = components["schemas"]["SdrTeamMember"];

// The periods the team review reads. A team is coached week by week, and a
// month is the longest span its handoff rounds stay comparable over.
const TEAM_PERIODS = [
  "this_week",
  "last_week",
  "this_month",
  "last_month",
] as const;
type TeamPeriod = (typeof TEAM_PERIODS)[number];

function teamPeriod(value: string | null | undefined): TeamPeriod {
  return TEAM_PERIODS.find((period) => period === value) ?? "this_week";
}

/** The team review of handoffs and members for one team and period. */
export function useSdrTeamReview(scope: AnalyticsScope, period: TeamPeriod) {
  const named = scope.kind === "team" || scope.kind === "workspace";
  return useQuery({
    queryKey: ["sdr-team", scope.kind, scope.id ?? "", period],
    queryFn: async ({ signal }): Promise<Review> =>
      unwrap(
        await api.GET("/analytics/sdr-team", {
          params: {
            query: named
              ? {
                  scope_kind: scope.kind as "team" | "workspace",
                  scope_id: scope.id ?? undefined,
                  period,
                }
              : { period },
          },
          signal,
        }),
      ),
  });
}

/**
 * Team prospecting: a team's outcomes this week, how its handoffs ended and
 * what still waits, and each member's share. The team's figures come from one
 * evaluation of the server's team preset and one team review. No row ranks a
 * member, and a small count is shown with its size.
 */
export function TeamProspecting({
  catalog,
  scope,
  workflow,
  switcher,
  scopeControl,
}: Readonly<{
  catalog: Catalog;
  scope: AnalyticsScope;
  workflow: UseQueryResult<Framework>;
  switcher: ReactNode;
  scopeControl: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [params] = useUrlParams();
  const period = teamPeriod(params.get("period"));
  const [evidence, setEvidence] = useState<ReportingEvidenceRef | null>(null);
  const { metrics, blocks } = presetRequest("team", catalog);
  const selection: ReportingSelection = {
    scope: { kind: scope.kind, id: scope.id ?? undefined, label: scope.label },
    period,
    target_basis: "month",
    close_window: "all_open",
    metrics,
    blocks: [],
  };
  const query = useReportingEvaluation(selection, metrics.length > 0);
  const review = useSdrTeamReview(scope, period);
  const handoffWorkflow =
    (workflow.data?.definition.template ??
      (review.data?.workflow === "handoff" ? "sdr" : undefined)) === "sdr";
  return (
    <>
      <FilterBar
        label={t("analytics.filters")}
        actions={
          <OverviewSaveActions
            selection={
              query.data ? { ...query.data.selection, blocks } : undefined
            }
          />
        }
        caption={resultsThrough(query.data, locale, t)}
      >
        {switcher}
        {scopeControl}
        <SegmentedControl
          label={t("reporting.period")}
          options={TEAM_PERIODS}
          value={period}
          onChange={(next) => {
            replaceDial("period", next);
            setEvidence(null);
          }}
          labels={{
            this_week: t("reporting.this_week"),
            last_week: t("reporting.last_week"),
            this_month: t("reporting.this_month"),
            last_month: t("reporting.last_month"),
          }}
        />
      </FilterBar>
      <QueryGate query={query} pendingLabel={t("analytics.overview.team")}>
        {(evaluation) => {
          const span = reportingPeriodLabel(
            evaluation.context.interval,
            evaluation.context.timezone,
            locale,
          );
          return (
            <>
              <StatStrip label={t("analytics.overview.readings")}>
                {(handoffWorkflow
                  ? (["meetings_held", "accepted_opportunities"] as const)
                  : (["meetings_held"] as const)
                ).map((id) => (
                  <MetricReading
                    key={id}
                    evaluation={evaluation}
                    id={id}
                    detail={span}
                    onEvidence={setEvidence}
                  />
                ))}
                <MetricReading
                  evaluation={evaluation}
                  id="leads_promoted"
                  detail={span}
                  onEvidence={setEvidence}
                />
                <MetricReading
                  evaluation={evaluation}
                  id="leads_awaiting_response"
                  onEvidence={setEvidence}
                />
              </StatStrip>
              <p className="t-caption">{t("analytics.team.countedOnce")}</p>
              {evidence && (
                <ReportingEvidenceDrawer
                  key={JSON.stringify(evidence)}
                  evaluation={evaluation}
                  reference={evidence}
                  onClose={() => setEvidence(null)}
                />
              )}
            </>
          );
        }}
      </QueryGate>
      <QueryGate query={review} pendingLabel={t("analytics.team.feedback")}>
        {(data) =>
          data.withheld ? (
            <SurfaceState
              state="withheld"
              emptyLabel={t("common.empty")}
              loadingLabel={t("analytics.team.feedback")}
              detail={{ withheldReason: t("analytics.team.withheld") }}
            >
              {null}
            </SurfaceState>
          ) : (
            <>
              {data.handoffs ? (
                <HandoffFeedback review={data} handoffs={data.handoffs} />
              ) : (
                <QualificationNote />
              )}
              <MemberTable review={data} />
            </>
          )
        }
      </QueryGate>
      <OverviewLinks
        sections={[
          { section: "sdr", label: t("reporting.sdr") },
          { section: "leads", label: t("reporting.leads") },
        ]}
      />
    </>
  );
}

// How the period's handoffs ended. Decisions made in the period and the rounds
// submitted in it are two questions, so they are two rows of figures.
function HandoffFeedback({
  review,
  handoffs,
}: Readonly<{ review: Review; handoffs: Handoffs }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const { outcomes, cohort, waiting } = handoffs;
  const asOf = Date.parse(review.as_of);
  const small = cohort.rounds < review.small_sample_below;
  const count = (value: number) => formatNumber(value, locale);
  return (
    <Panel title={t("analytics.team.feedback")}>
      <PanelBody>
        <StatStrip label={t("analytics.team.decided")}>
          <StatCard
            narrow="row"
            label={t("analytics.team.submitted")}
            value={count(outcomes.submitted)}
            detail={t("analytics.team.submittedDetail")}
          />
          <StatCard
            narrow="row"
            label={t("analytics.team.accepted")}
            value={count(outcomes.accepted)}
            detail={t("analytics.team.acceptedDetail")}
          />
          <StatCard
            narrow="row"
            label={t("analytics.team.returned")}
            value={count(outcomes.recycled)}
            detail={t("analytics.team.returnedDetail")}
          />
          <StatCard
            narrow="row"
            label={t("analytics.team.rejected")}
            value={count(outcomes.rejected)}
            detail={t("analytics.team.rejectedDetail")}
          />
        </StatStrip>
        <p>
          {plural("analytics.team.cohort", cohort.rounds, {
            count: count(cohort.rounds),
            accepted: count(cohort.accepted),
            returned: count(cohort.recycled),
            rejected: count(cohort.rejected),
            waiting: count(cohort.waiting),
          })}{" "}
          {cohort.median_decision_hours != null &&
            t("analytics.team.medianDecision", {
              hours: formatHours(cohort.median_decision_hours, locale),
            })}
        </p>
        {small && cohort.rounds > 0 && (
          <Callout
            tone="info"
            kind="standing"
            title={t("analytics.team.smallSampleTitle")}
          >
            <p>
              {t("analytics.team.smallSample", {
                below: count(review.small_sample_below),
              })}
            </p>
          </Callout>
        )}
        <p>
          {plural("analytics.team.waitingNow", waiting.waiting, {
            count: count(waiting.waiting),
          })}{" "}
          {waiting.oldest_waiting_since &&
            t("analytics.sdrLongestWait", {
              age: formatDuration(
                asOf - Date.parse(waiting.oldest_waiting_since),
                locale,
              ),
            })}{" "}
          {waiting.blocked > 0 &&
            plural("analytics.team.blocked", waiting.blocked, {
              count: count(waiting.blocked),
            })}
        </p>
        {handoffs.reasons.length > 0 && (
          <ReasonList reasons={handoffs.reasons} locale={locale} />
        )}
        {handoffs.unattributed > 0 && (
          <p className="t-caption">
            {plural("analytics.team.unattributed", handoffs.unattributed, {
              count: count(handoffs.unattributed),
            })}
          </p>
        )}
      </PanelBody>
    </Panel>
  );
}

function ReasonList({
  reasons,
  locale,
}: Readonly<{ reasons: Handoffs["reasons"]; locale: Locale }>) {
  const t = useT();
  return (
    <DataTable
      label={t("analytics.team.reasons")}
      rows={[...reasons]}
      rowKey={(reason) => `${reason.outcome}:${reason.label}`}
      columns={[
        {
          key: "reason",
          header: t("analytics.team.reason"),
          grow: true,
          render: (reason) => reason.label,
        },
        {
          key: "outcome",
          header: t("analytics.team.outcome"),
          render: (reason) =>
            t(
              reason.outcome === "rejected"
                ? "analytics.team.rejected"
                : "analytics.team.returned",
            ),
        },
        {
          key: "count",
          header: t("analytics.team.count"),
          align: "end",
          render: (reason) => formatNumber(reason.count, locale),
        },
      ]}
    />
  );
}

// One row per member, in name order: what each one's work came to, beside the
// work still waiting on others. The rows are not a ranking.
function MemberTable({ review }: Readonly<{ review: Review }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (review.members_withheld) {
    return <p className="t-caption">{t("analytics.team.membersWithheld")}</p>;
  }
  if (review.members.length === 0) return null;
  const asOf = Date.parse(review.as_of);
  const count = (value: number) => formatNumber(value, locale);
  const handoffColumns = review.handoffs
    ? [
        {
          key: "submitted",
          header: t("analytics.team.submitted"),
          align: "end" as const,
          render: (row: Member) => count(row.submitted),
        },
        {
          key: "accepted",
          header: t("analytics.team.accepted"),
          align: "end" as const,
          render: (row: Member) => count(row.accepted),
        },
        {
          key: "returned",
          header: t("analytics.team.returned"),
          align: "end" as const,
          render: (row: Member) => count(row.recycled),
        },
        {
          key: "rejected",
          header: t("analytics.team.rejected"),
          align: "end" as const,
          render: (row: Member) => count(row.rejected),
        },
        {
          key: "waiting",
          header: t("analytics.team.waiting"),
          align: "end" as const,
          render: (row: Member) =>
            row.oldest_waiting_since
              ? plural("analytics.team.waitingFor", row.waiting, {
                  count: count(row.waiting),
                  age: formatDuration(
                    asOf - Date.parse(row.oldest_waiting_since),
                    locale,
                  ),
                })
              : count(row.waiting),
        },
      ]
    : [];
  return (
    <Panel title={t("analytics.team.members")}>
      <PanelBody>
        <DataTable
          label={t("analytics.team.members")}
          rows={[...review.members]}
          rowKey={(row) => row.id}
          columns={[
            {
              key: "member",
              header: t("analytics.team.member"),
              grow: true,
              render: (row) => row.label,
            },
            {
              key: "meetings",
              header: t("analytics.team.meetings"),
              align: "end",
              render: (row) => count(row.meetings_held),
            },
            ...handoffColumns,
          ]}
        />
        <p className="t-caption">{t("analytics.team.rowsOverlap")}</p>
        {review.members_truncated && (
          <p className="t-caption">{t("analytics.team.truncated")}</p>
        )}
      </PanelBody>
    </Panel>
  );
}
