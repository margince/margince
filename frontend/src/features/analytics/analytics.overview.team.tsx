// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type UseQueryResult, useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { SegmentedControl, StatCard } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { FilterBar } from "../../design-system/filterbar";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { StatStrip } from "../../design-system/statstrip";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatDuration, formatHours, formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { replaceDial, useUrlParams } from "../../lib/urlstate";
import { type AnalyticsScope, scopeKey } from "./analytics.context";
import { outboundDials, TEAM_PERIODS } from "./analytics.overview.context";
import { QualificationNote } from "./analytics.overview.prospecting";
import {
  MetricReading,
  OverviewLinks,
  OverviewSaveActions,
  useSelectionEvidence,
} from "./analytics.overview.readings";
import { MemberTable, ReasonList } from "./analytics.overview.team.members";
import { periodCaption } from "./analytics.period";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import {
  type ReportingEvidenceRef,
  type ReportingSelection,
  reportingPeriodLabel,
} from "./reporting.model";
import { useReportingEvaluation } from "./reporting.queries";
import { presetRequest } from "./reporting.views";

type Catalog = components["schemas"]["ReportingCatalog"];
type Framework = components["schemas"]["ReportingFramework"];
type Review = components["schemas"]["SdrTeamReview"];
type Handoffs = components["schemas"]["SdrTeamHandoffs"];

type TeamPeriod = (typeof TEAM_PERIODS)[number];

function teamPeriod(value: string | null | undefined): TeamPeriod {
  return TEAM_PERIODS.find((period) => period === value) ?? "this_week";
}

type TeamScope = Readonly<{
  kind: "team" | "workspace" | "managed_teams";
  id?: string;
}>;

// The scope a team review may read. A colleague's own records are not a team,
// so an owner scope has none and the page reads nothing for it.
function teamScope(scope: AnalyticsScope): TeamScope | undefined {
  return scope.kind === "owner"
    ? undefined
    : { kind: scope.kind, id: scope.id ?? undefined };
}

/** The team review of handoffs and members for one team and period. */
export function useSdrTeamReview(
  scope: TeamScope | undefined,
  period: TeamPeriod,
) {
  return useQuery({
    queryKey: ["sdr-team", scope?.kind, scope?.id ?? "", period],
    enabled: scope !== undefined,
    queryFn: async ({ signal }): Promise<Review> =>
      unwrap(
        await api.GET("/analytics/sdr-team", {
          params: {
            query: { scope_kind: scope?.kind, scope_id: scope?.id, period },
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
  defaultScope,
  workflow,
  switcher,
  scopeControl,
}: Readonly<{
  catalog: Catalog;
  scope: AnalyticsScope;
  defaultScope: AnalyticsScope;
  workflow: UseQueryResult<Framework>;
  switcher: ReactNode;
  scopeControl: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [params] = useUrlParams();
  const period = teamPeriod(params.get("period"));
  const { metrics, blocks } = presetRequest("team", catalog);
  // Both reads take this one scope, so the figures and the feedback below
  // them always describe the same members.
  const team = teamScope(scope);
  const selection: ReportingSelection = {
    scope: { kind: scope.kind, id: scope.id ?? undefined, label: scope.label },
    period,
    target_basis: "month",
    close_window: "all_open",
    metrics,
    blocks: [],
  };
  const query = useReportingEvaluation(
    selection,
    team !== undefined && metrics.length > 0,
  );
  const review = useSdrTeamReview(team, period);
  const { evidence, openEvidence, closeEvidence } = useSelectionEvidence(
    JSON.stringify(selection),
  );
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
        caption={periodCaption(query.data, locale, t)}
      >
        {switcher}
        {scopeControl}
        <SegmentedControl
          label={t("reporting.period")}
          options={TEAM_PERIODS}
          value={period}
          onChange={(next) => replaceDial("period", next)}
          labels={{
            this_week: t("reporting.this_week"),
            last_week: t("reporting.last_week"),
            this_month: t("reporting.this_month"),
            last_month: t("reporting.last_month"),
          }}
        />
      </FilterBar>
      {team ? (
        <TeamBody
          query={query}
          review={review}
          handoffWorkflow={handoffWorkflow}
          evidence={evidence}
          onEvidence={openEvidence}
          onCloseEvidence={closeEvidence}
        />
      ) : (
        <Callout
          tone="info"
          kind="standing"
          title={t("analytics.team.chooseTeam")}
        >
          <p>{t("analytics.team.chooseTeamBody")}</p>
        </Callout>
      )}
      <OverviewLinks
        dials={outboundDials(
          "team",
          { scope: scopeKey(scope), period },
          scopeKey(defaultScope),
        )}
        sections={[
          { section: "sdr", label: t("reporting.sdr") },
          { section: "leads", label: t("reporting.leads") },
        ]}
      />
    </>
  );
}

// The team's readings, then its handoff feedback and members.
function TeamBody({
  query,
  review,
  handoffWorkflow,
  evidence,
  onEvidence,
  onCloseEvidence,
}: Readonly<{
  query: ReturnType<typeof useReportingEvaluation>;
  review: ReturnType<typeof useSdrTeamReview>;
  handoffWorkflow: boolean;
  evidence: ReportingEvidenceRef | null;
  onEvidence: (reference: ReportingEvidenceRef) => void;
  onCloseEvidence: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <>
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
                    onEvidence={onEvidence}
                  />
                ))}
                <MetricReading
                  evaluation={evaluation}
                  id="leads_promoted"
                  detail={span}
                  onEvidence={onEvidence}
                />
                <MetricReading
                  evaluation={evaluation}
                  id="leads_awaiting_response"
                  onEvidence={onEvidence}
                />
              </StatStrip>
              <p className="t-caption">{t("analytics.team.countedOnce")}</p>
              {evidence && (
                <ReportingEvidenceDrawer
                  key={JSON.stringify(evidence)}
                  evaluation={evaluation}
                  reference={evidence}
                  onClose={onCloseEvidence}
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
