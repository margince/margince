// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { Button, StatCard } from "../../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { StatStrip } from "../../design-system/statstrip";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatDate, formatDateTime, formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { UPCOMING_LIST_ID } from "./analytics.overview.prospectinghero";
import {
  type ReportingEvaluation,
  reportingPeriodLabel,
} from "./reporting.model";

type SdrSummary = components["schemas"]["SdrSummary"];
type Meeting = components["schemas"]["SdrUpcomingMeeting"];
type TeamReview = components["schemas"]["SdrTeamReview"];

// The periods the handoff review reads; any other period has no cohort here.
const COHORT_PERIODS = [
  "this_week",
  "last_week",
  "this_month",
  "last_month",
] as const;
type CohortPeriod = (typeof COHORT_PERIODS)[number];

/**
 * The meetings booked ahead, in the window the summary counts them in, so the
 * list's pages hold that count. Each opens the record it is with, to prepare.
 * A meeting is listed by its start, in the reader's own clock.
 */
export function UpcomingMeetings({
  self,
  summary,
  title,
}: Readonly<{
  self: string;
  summary: Readonly<{ data?: SdrSummary; isError: boolean }>;
  title: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const count = summary.data?.upcoming_meetings;
  const rows = useInfiniteQuery({
    queryKey: ["sdr-summary", self, "meetings"],
    enabled: count != null && count > 0,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam, signal }) =>
      unwrap(
        await api.GET("/analytics/sdr-summary/meetings", {
          params: {
            query: {
              scope_kind: "owner",
              scope_id: self,
              ...(pageParam ? { cursor: pageParam } : {}),
            },
          },
          signal,
        }),
      ),
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
  const loaded = rows.data?.pages.flatMap((page) => page.rows) ?? [];
  const state = summary.isError
    ? "failed"
    : !summary.data
      ? "loading"
      : count == null
        ? "withheld"
        : count === 0
          ? "empty"
          : "ready";
  return (
    <div id={UPCOMING_LIST_ID} tabIndex={-1}>
      <Panel title={title} flatHead>
        {state !== "ready" && (
          <PanelBody>
            <SurfaceState
              state={state}
              emptyLabel={t("analytics.prospect.noUpcoming")}
              loadingLabel={title}
            >
              {null}
            </SurfaceState>
          </PanelBody>
        )}
        {state === "ready" && (
          <QueryGate query={rows} pendingLabel={title}>
            {() => (
              <>
                {loaded.map((meeting) => (
                  <MeetingRow
                    key={meeting.id}
                    meeting={meeting}
                    locale={locale}
                  />
                ))}
                {rows.hasNextPage && (
                  <PanelRow>
                    <Button
                      onClick={() => void rows.fetchNextPage()}
                      disabled={rows.isFetchingNextPage}
                    >
                      {t("analytics.sdrShowMore")}
                    </Button>
                  </PanelRow>
                )}
              </>
            )}
          </QueryGate>
        )}
      </Panel>
    </div>
  );
}

function MeetingRow({
  meeting,
  locale,
}: Readonly<{
  meeting: Meeting;
  locale: ReturnType<typeof useLocale>["locale"];
}>) {
  const t = useT();
  // A meeting known only by its day names the day, never a made-up noon.
  const when =
    meeting.starts_precision === "day"
      ? formatDate(meeting.starts_at, locale, "UTC")
      : formatDateTime(meeting.starts_at, locale, viewerZone());
  return (
    <PanelRow className="analytics-attention-row">
      <div className="analytics-attention-text">
        <p className="analytics-attention-title">{meeting.title}</p>
        <p className="t-caption">
          {when}
          {" · "}
          {meeting.record ? (
            <EntityRef
              kind={meeting.record.type}
              id={meeting.record.id}
              name={meeting.record.label}
            />
          ) : (
            t("analytics.prospect.restrictedRecord")
          )}
        </p>
      </div>
    </PanelRow>
  );
}

/**
 * What happened to the handoffs the reader submitted in the period. The cohort
 * is named. Each round counts once, in the outcome it reached when read.
 * Counts only; no rate is drawn from them.
 */
export function HandoffCohort({
  self,
  evaluation,
}: Readonly<{ self: string; evaluation: ReportingEvaluation }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const period = COHORT_PERIODS.find(
    (each) => each === evaluation.context.period_kind,
  );
  const review = useOwnReview(self, period);
  const title = t("analytics.prospect.cohortTitle");
  if (!period)
    return (
      <Panel title={title} flatHead>
        <PanelBody>
          <p className="t-caption">{t("analytics.prospect.cohortPeriods")}</p>
        </PanelBody>
      </Panel>
    );
  return (
    <Panel title={title} flatHead>
      <PanelBody>
        <QueryGate query={review} pendingLabel={title}>
          {(answer) => {
            const cohort = answer.handoffs?.cohort;
            if (!cohort)
              return (
                <p className="t-caption">{t("analytics.prospect.withheld")}</p>
              );
            const when = reportingPeriodLabel(
              answer.interval,
              evaluation.context.timezone,
              locale,
            );
            return (
              <>
                <p className="t-caption">
                  {plural("analytics.prospect.cohortCaption", cohort.rounds, {
                    count: formatNumber(cohort.rounds, locale),
                    period: when,
                  })}
                </p>
                {cohort.rounds > 0 && (
                  <StatStrip label={title}>
                    {(
                      [
                        ["accepted", cohort.accepted],
                        ["recycled", cohort.recycled],
                        ["rejected", cohort.rejected],
                        ["waiting", cohort.waiting],
                      ] as const
                    ).map(([outcome, count]) => (
                      <StatCard
                        key={outcome}
                        narrow="row"
                        label={t(`analytics.prospect.cohort.${outcome}`)}
                        value={formatNumber(count, locale)}
                      />
                    ))}
                  </StatStrip>
                )}
              </>
            );
          }}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}

// The reader's own handoff review: the shared team reader, for one seat.
function useOwnReview(self: string, period: CohortPeriod | undefined) {
  return useQuery({
    queryKey: ["sdr-team", "owner", self, period],
    enabled: period !== undefined,
    queryFn: async ({ signal }): Promise<TeamReview> =>
      unwrap(
        await api.GET("/analytics/sdr-team", {
          params: {
            query: { scope_kind: "owner", scope_id: self, period },
          },
          signal,
        }),
      ),
  });
}
