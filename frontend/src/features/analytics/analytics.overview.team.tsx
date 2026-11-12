// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type UseQueryResult, useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { Suspense } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Field, PendingBody } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { FilterBar } from "../../design-system/filterbar";
import { OverflowMenu } from "../../design-system/overflowmenu";
import { QueryGate } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { StatStrip } from "../../design-system/statstrip";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatAnalyticsNumber, formatDateAbbrev } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { useCompanyZone } from "../../lib/companyzone";
import { unwrap } from "../../lib/problem";
import { replaceDial, useUrlParams } from "../../lib/urlstate";
import { AnalyticsBoard, AnalyticsSheet } from "./analytics.board";
import { type AnalyticsScope, scopeKey } from "./analytics.context";
import { ReportingEvidenceDrawer } from "./analytics.drawers";
import type { HandoffScope } from "./analytics.outcomes.handoffpages";
import { TEAM_PERIODS } from "./analytics.overview.context";
import { useOverviewNavigation } from "./analytics.overview.navigation";
import { QualificationNote } from "./analytics.overview.prospecting";
import { teamPeriod, teamQuestion } from "./analytics.overview.questions";
import {
  MetricReading,
  OverviewSaveActions,
} from "./analytics.overview.readings";
import {
  TeamStanding,
  useTeamSummary,
} from "./analytics.overview.team.standing";
import { AssessmentPanel, MemberTable } from "./analytics.overview.work";
import { periodCaption } from "./analytics.period";
import { useSelectionEvidence } from "./reporting.evidenceselection";
import { ReportingExportButton } from "./reporting.export";
import {
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  type ReportingMetricID,
  reportingPeriodLabel,
} from "./reporting.model";
import { useReportingEvaluation } from "./reporting.queries";

type Catalog = components["schemas"]["ReportingCatalog"];
type Framework = components["schemas"]["ReportingFramework"];
type Review = components["schemas"]["SdrTeamReview"];

type TeamPeriod = (typeof TEAM_PERIODS)[number];

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
  // Both reads take this one scope, so the figures and the feedback below
  // them always describe the same members.
  const team = teamScope(scope);
  const review = useSdrTeamReview(team, period);
  const template =
    workflow.data?.definition.template ??
    (review.data
      ? review.data.workflow === "handoff"
        ? "sdr"
        : "sales"
      : undefined);
  const { selection, blocks } = teamQuestion(catalog, scope, period, template);
  const { metrics } = selection;
  useOverviewNavigation("team", selection);
  const query = useReportingEvaluation(
    selection,
    team !== undefined &&
      metrics.length > 0 &&
      (Boolean(template) || (!workflow.isLoading && !review.isLoading)),
  );
  const { evidence, evidenceFrom, openEvidence, closeEvidence } =
    useSelectionEvidence(JSON.stringify(selection), query.data);
  const handoffWorkflow =
    (workflow.data?.definition.template ??
      (review.data?.workflow === "handoff" ? "sdr" : undefined)) === "sdr";
  return (
    <>
      <FilterBar
        label={t("analytics.filters")}
        quiet
        labels="inside"
        caption={periodCaption(query.data, locale, t)}
      >
        {switcher}
        {scopeControl}
        <Field label={t("reporting.period")}>
          {(field) => (
            <Select
              {...field}
              value={period}
              options={TEAM_PERIODS.map((value) => ({
                value,
                label: t(`reporting.${value}`),
              }))}
              onChange={(next) => replaceDial("period", teamPeriod(next))}
            />
          )}
        </Field>
        <OverflowMenu label={t("analytics.overview.more")}>
          <OverviewSaveActions
            selection={
              query.data ? { ...query.data.selection, blocks } : undefined
            }
            scope={scopeKey(scope)}
          />
          {query.data && <ReportingExportButton evaluation={query.data} />}
        </OverflowMenu>
      </FilterBar>
      {team ? (
        <TeamBody
          query={query}
          review={review}
          team={team}
          handoffWorkflow={handoffWorkflow}
          evidence={evidence}
          evidenceFrom={evidenceFrom}
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
    </>
  );
}

// The team's readings and members in the column; the outlook and what holds
// the team up in the rail. The readings lead with the workflow's own outcome.
function TeamBody({
  query,
  review,
  team,
  handoffWorkflow,
  evidence,
  evidenceFrom,
  onEvidence,
  onCloseEvidence,
}: Readonly<{
  query: ReturnType<typeof useReportingEvaluation>;
  review: ReturnType<typeof useSdrTeamReview>;
  team: TeamScope;
  handoffWorkflow: boolean;
  evidence: ReportingEvidenceRef | null;
  evidenceFrom: ReportingEvaluation | undefined;
  onEvidence: (reference: ReportingEvidenceRef) => void;
  onCloseEvidence: () => void;
}>) {
  const t = useT();
  const scope = handoffScope(team);
  const summary = useTeamSummary(scope, true);
  // The company's clock until the evaluation names the reporting zone.
  const recordZone = useCompanyZone();
  const zone = query.data?.context.timezone ?? recordZone;
  const readings = (
    handoffWorkflow ? HANDOFF_READINGS : QUALIFICATION_READINGS
  ).map((id) =>
    id === "leads_qualified" &&
    !query.data?.metrics.some((metric) => metric.id === "leads_qualified")
      ? "leads_promoted"
      : id,
  );
  const withheld = review.data?.withheld;
  return (
    <AnalyticsBoard
      railLabel={t("analytics.board.rail")}
      main={
        <AnalyticsSheet>
          <QueryGate query={query} pendingLabel={t("analytics.overview.team")}>
            {(evaluation) => (
              <>
                <TeamReadings
                  evaluation={evaluation}
                  readings={readings}
                  onEvidence={onEvidence}
                />
                {evidence && (
                  <ReportingEvidenceDrawer
                    key={JSON.stringify(evidence)}
                    evaluation={evidenceFrom ?? evaluation}
                    reference={evidence}
                    onClose={onCloseEvidence}
                  />
                )}
              </>
            )}
          </QueryGate>
          <QueryGate query={review} pendingLabel={t("analytics.team.members")}>
            {(data) =>
              data.withheld ? (
                <SurfaceState
                  state="withheld"
                  emptyLabel={t("common.empty")}
                  loadingLabel={t("analytics.team.standing")}
                  detail={{ withheldReason: data.withheld_reason }}
                >
                  {null}
                </SurfaceState>
              ) : (
                <>
                  <Suspense
                    fallback={<PendingBody label={t("common.loading")} />}
                  >
                    <MemberTable
                      review={data}
                      zone={zone}
                      onOpen={(id, owner) => {
                        const metric = query.data?.metrics.find(
                          (metric) => metric.id === id,
                        );
                        if (metric)
                          onEvidence({
                            ...metric.evidence,
                            group_key: `owner:${owner}`,
                          });
                      }}
                    />
                  </Suspense>
                  {data.workflow === "qualification" && <QualificationNote />}
                  {data.handoffs && data.handoffs.unattributed > 0 && (
                    <Unattributed count={data.handoffs.unattributed} />
                  )}
                </>
              )
            }
          </QueryGate>
        </AnalyticsSheet>
      }
      rail={
        <>
          {query.data && !query.isError && !withheld && (
            <Suspense fallback={<PendingBody label={t("common.loading")} />}>
              <AssessmentPanel
                view="team"
                work={review.data?.members_withheld ? undefined : summary.data}
                workPending={summary.isPending || review.isPending}
                selection={query.data.selection}
                evaluation={query.data}
                onEvidence={onEvidence}
              />
            </Suspense>
          )}
          {review.data && !withheld && (
            <TeamStanding
              review={review.data}
              readings={query}
              summary={summary}
              scope={scope}
              zone={zone}
              onEvidence={onEvidence}
            />
          )}
        </>
      }
    />
  );
}

// The team's readings as one plate. The waiting figure is read now, so it
// names the moment and zone; the rest name the period.
function TeamReadings({
  evaluation,
  readings,
  onEvidence,
}: Readonly<{
  evaluation: ReportingEvaluation;
  readings: readonly ReportingMetricID[];
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const { interval, evaluated_at, timezone } = evaluation.context;
  const span = reportingPeriodLabel(interval, timezone, locale);
  return (
    <StatStrip label={t("analytics.overview.readings")} density="plate">
      {readings.map((id) => (
        <MetricReading
          key={id}
          evaluation={evaluation}
          id={id}
          detail={
            id === "leads_awaiting_response_now"
              ? t("analytics.frame", {
                  asOf: formatDateAbbrev(evaluated_at, locale, timezone),
                  zone: timezone,
                })
              : span
          }
          onEvidence={onEvidence}
        />
      ))}
    </StatStrip>
  );
}

// Each figure counts the team's work once; the team's readings count a
// meeting several members attended once.
const HANDOFF_READINGS = [
  "accepted_opportunities",
  "meetings_held",
  "leads_awaiting_response_now",
] as const;
const QUALIFICATION_READINGS = [
  "leads_qualified",
  "meetings_held",
  "leads_awaiting_response_now",
] as const;

// The handoff list's scope for a team review. My teams names no kind the list
// takes, so it is the seat's own default there too.
function handoffScope(team: TeamScope): HandoffScope {
  return team.kind === "managed_teams" ? {} : { kind: team.kind, id: team.id };
}

function Unattributed({ count }: Readonly<{ count: number }>) {
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <p className="t-caption">
      {plural("analytics.team.unattributed", count, {
        count: formatAnalyticsNumber(count, locale),
      })}
    </p>
  );
}
