// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type UseQueryResult, useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Field } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { FilterBar } from "../../design-system/filterbar";
import { OverflowMenu } from "../../design-system/overflowmenu";
import { QueryGate } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { StatStrip } from "../../design-system/statstrip";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { replaceDial, useUrlParams } from "../../lib/urlstate";
import { type AnalyticsScope, scopeKey } from "./analytics.context";
import type { HandoffScope } from "./analytics.outcomes.sdr";
import { outboundDials, TEAM_PERIODS } from "./analytics.overview.context";
import { QualificationNote } from "./analytics.overview.prospecting";
import {
  MetricReading,
  OverviewLinks,
  OverviewSaveActions,
} from "./analytics.overview.readings";
import { MemberTable } from "./analytics.overview.team.members";
import {
  TeamStanding,
  useTeamSummary,
} from "./analytics.overview.team.standing";
import { periodCaption } from "./analytics.period";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import { useSelectionEvidence } from "./reporting.evidenceselection";
import { ReportingExportButton } from "./reporting.export";
import {
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  type ReportingSelection,
  reportingPeriodLabel,
} from "./reporting.model";
import { useReportingEvaluation } from "./reporting.queries";
import { presetRequest } from "./reporting.views";

type Catalog = components["schemas"]["ReportingCatalog"];
type Framework = components["schemas"]["ReportingFramework"];
type Review = components["schemas"]["SdrTeamReview"];

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
      <OverviewLinks
        dials={outboundDials("team", { scope: scopeKey(scope), period })}
        sections={[
          { section: "sdr", label: t("reporting.sdr") },
          { section: "leads", label: t("reporting.leads") },
        ]}
      />
    </>
  );
}

// The team's outcomes, then where it stands and what holds it up, then its
// members. The readings lead with the workflow's own outcome.
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
  const { locale } = useLocale();
  const scope = handoffScope(team);
  const summary = useTeamSummary(scope, true);
  // The company's clock until the evaluation names the reporting zone.
  const recordZone = useRecordZone();
  const zone = query.data?.context.timezone ?? recordZone;
  const readings = handoffWorkflow ? HANDOFF_READINGS : QUALIFICATION_READINGS;
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
                {readings.map((id) => (
                  <MetricReading
                    key={id}
                    evaluation={evaluation}
                    id={id}
                    detail={span}
                    onEvidence={onEvidence}
                  />
                ))}
              </StatStrip>
              {evidence && (
                <ReportingEvidenceDrawer
                  key={JSON.stringify(evidence)}
                  evaluation={evidenceFrom ?? evaluation}
                  reference={evidence}
                  onClose={onCloseEvidence}
                />
              )}
            </>
          );
        }}
      </QueryGate>
      <QueryGate query={review} pendingLabel={t("analytics.team.standing")}>
        {(data) =>
          data.withheld ? (
            <SurfaceState
              state="withheld"
              emptyLabel={t("common.empty")}
              loadingLabel={t("analytics.team.standing")}
              detail={{ withheldReason: t("analytics.team.withheld") }}
            >
              {null}
            </SurfaceState>
          ) : (
            <>
              <div className="analytics-pair analytics-pair-table">
                <TeamStanding
                  review={data}
                  readings={query}
                  summary={summary}
                  scope={scope}
                  zone={zone}
                  onEvidence={onEvidence}
                />
                <MemberTable review={data} zone={zone} />
              </div>
              {data.workflow === "qualification" && <QualificationNote />}
              {data.handoffs && data.handoffs.unattributed > 0 && (
                <Unattributed count={data.handoffs.unattributed} />
              )}
            </>
          )
        }
      </QueryGate>
    </>
  );
}

// Each figure counts the team's work once; the team's readings count a
// meeting several members attended once.
const HANDOFF_READINGS = [
  "accepted_opportunities",
  "meetings_held",
  "leads_promoted",
  "leads_awaiting_response",
] as const;
const QUALIFICATION_READINGS = [
  "leads_promoted",
  "meetings_held",
  "leads_awaiting_response",
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
        count: formatNumber(count, locale),
      })}
    </p>
  );
}
