// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useUrlParams } from "../app/urlstate";
import { Disclosure } from "../design-system/atoms";
import { PageZones } from "../design-system/pagezones";
import { useNow } from "../format/now";
import { useT } from "../i18n";
import { changedSinceBrief } from "./brief.changed";
import { BriefDials } from "./brief.dials";
import { briefDay } from "./brief.facts";
import { BriefFeed } from "./brief.feed";
import { BriefGlance } from "./brief.glance";
import { PlanSection } from "./brief.plan";
import { useWeeklyReview } from "./brief.queries";
import { OvernightPanel } from "./brief.rail.overnight";
import { BriefReadingsStrip } from "./brief.readings";
import { SchedulePanel } from "./brief.schedule";
import { BriefTeamBoard } from "./brief.teamboard";
import { BriefTeamSelect } from "./brief.teamselect";
import { TeamWeeklyPanel } from "./brief.teamweekly";
import { addressFrom, type BriefAddress, paramsFor } from "./brief.view";
import { WeeklySection } from "./brief.weekly";
import { BriefCoverage } from "./briefcoverage";
import { useMe } from "./common";
import { useWorklist } from "./worklist.queries";
import "./brief.css";

export function BriefScreen() {
  const nowMs = useNow(60_000);
  const me = useMe();
  const own = useWorklist("mine", "all");
  const teamOffered =
    own.data?.pages[0]?.scope_options?.includes("team") ?? false;
  const [params, setParams] = useUrlParams();
  const address = addressFrom(params, teamOffered);
  const query = own;
  const day = briefDay(query.data?.pages);
  const review = useWeeklyReview(address.week);
  const firstName = me.data?.user?.display_name?.trim().split(/\s+/)[0] ?? null;
  return (
    <div className="wrap brief-wrap">
      <div className="brief-head">
        <BriefGlance
          view={address.view}
          scope={address.scope}
          day={address.scope === "mine" ? day : undefined}
          week={address.scope === "mine" ? review.data : undefined}
          firstName={firstName}
          now={new Date(nowMs)}
        />
        <BriefDials
          address={address}
          offered={teamOffered}
          onChange={(next) => setParams(paramsFor(next))}
        />
      </div>
      <BriefBody address={address} teamOffered={teamOffered} query={query} />
    </div>
  );
}

export { deckItems } from "./brief.decisions.items";

function BriefBody({
  address,
  teamOffered,
  query,
}: Readonly<{
  address: BriefAddress;
  teamOffered: boolean;
  query: ReturnType<typeof useWorklist>;
}>) {
  if (address.view === "weekly")
    return address.scope === "team" ? (
      <TeamWeeklyPanel offered={teamOffered} />
    ) : (
      <>
        <WeeklySection />
        <PlanSection />
      </>
    );
  if (address.scope === "team")
    return (
      <BriefTeamSelect>
        {(teamId) => <BriefTeamBoard offered={teamOffered} teamId={teamId} />}
      </BriefTeamSelect>
    );
  return <PersonalMorning query={query} />;
}

function PersonalMorning({
  query,
}: Readonly<{ query: ReturnType<typeof useWorklist> }>) {
  const t = useT();
  const day = briefDay(query.data?.pages);
  const state = query.isPending
    ? "loading"
    : query.isError && !day
      ? "failed"
      : "ready";
  return (
    <>
      {day && (
        <Disclosure
          summary={t("brief.readings.summary")}
          className="brief-summary"
        >
          <BriefReadingsStrip day={day} />
        </Disclosure>
      )}
      {day && <BriefCoverage day={day} onRetry={() => void query.refetch()} />}
      <PageZones
        shape="aside"
        mainClassName="brief-main"
        asideClassName="brief-rail"
        asideLabel={t("brief.rail")}
        main={
          <BriefFeed
            day={day}
            state={state}
            changed={changedSinceBrief(day)}
            onMore={() => void query.fetchNextPage()}
            loadingMore={query.isFetchingNextPage}
            moreFailed={query.isFetchNextPageError}
            refreshFailed={query.isRefetchError}
            onRetry={() => void query.refetch()}
          />
        }
        aside={
          <>
            <SchedulePanel day={day} state={state} />
            <OvernightPanel />
          </>
        }
      />
    </>
  );
}
