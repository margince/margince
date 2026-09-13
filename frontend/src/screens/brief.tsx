// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useLayoutEffect, useRef, useState } from "react";
import { navigate } from "../app/router";
import { useUrlParams } from "../app/urlstate";
import { Button } from "../design-system/atoms";
import { PageZones } from "../design-system/pagezones";
import { Panel, PanelBody } from "../design-system/panel";
import { formatDateTime } from "../format/format";
import { useNow } from "../format/now";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { changedSinceBrief } from "./brief.changed";
import { BriefChanges } from "./brief.changes";
import { BriefDials } from "./brief.dials";
import { briefDay } from "./brief.facts";
import { BriefFeed, BriefUpdates } from "./brief.feed";
import { BriefGlance } from "./brief.glance";
import { PlanSection } from "./brief.plan";
import { useMorningBrief, useWeeklyReview } from "./brief.queries";
import { BriefQueue } from "./brief.queue";
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
import { WorklistPane } from "./worklist.pane";
import { useWorklist } from "./worklist.queries";
import "./brief.css";

export function BriefScreen() {
  const t = useT();
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
          onChange={(next) => setParams(paramsFor(next, params))}
        />
      </div>
      <Button
        onClick={() => {
          const next = new Map(params);
          next.set("queue", "1");
          navigate({ screen: "brief" }, next);
        }}
      >
        {t("brief.queue.open")}
      </Button>
      <BriefBody address={address} teamOffered={teamOffered} query={query} />
      <BriefQueue />
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
  const { locale } = useLocale();
  const brief = useMorningBrief();
  const [selected, setSelected] = useState("");
  const contextRegion = useRef<HTMLDivElement>(null);
  const contextOpener = useRef<HTMLElement | null>(null);
  const day = briefDay(query.data?.pages);
  const context = day?.focus?.items.find(
    (item) => `${item.source}-${item.id}` === selected,
  );
  const contextIdentity = context ? `${context.source}-${context.id}` : "";
  useLayoutEffect(() => {
    if (contextIdentity) contextRegion.current?.focus();
    else if (contextOpener.current) {
      contextOpener.current.focus();
      contextOpener.current = null;
    }
  }, [contextIdentity]);
  const state = query.isPending
    ? "loading"
    : query.isError && !day
      ? "failed"
      : "ready";
  return (
    <PageZones
      shape="aside"
      className={context ? "brief-context-open" : undefined}
      mainClassName="brief-main"
      asideClassName="brief-rail"
      asideLabel={t("brief.rail")}
      main={
        <BriefFeed
          day={day}
          onContext={(item) => {
            contextOpener.current =
              document.activeElement instanceof HTMLElement
                ? document.activeElement
                : null;
            setSelected(`${item.source}-${item.id}`);
          }}
          state={state}
          changed={changedSinceBrief(day)}
          refreshFailed={query.isRefetchError}
          onRetry={() => void query.refetch()}
        />
      }
      aside={
        context ? (
          <div ref={contextRegion} tabIndex={-1}>
            <Button onClick={() => setSelected("")}>
              {t("brief.focus.back")}
            </Button>
            <WorklistPane item={context} />
          </div>
        ) : (
          <>
            {day && (
              <Panel
                title={t("brief.readings.summary")}
                className="brief-summary"
              >
                <PanelBody>
                  {brief.data?.generated_at && (
                    <p className="t-caption">
                      {t("brief.createdAt", {
                        when: formatDateTime(
                          brief.data.generated_at,
                          locale,
                          viewerZone(),
                        ),
                      })}
                    </p>
                  )}
                  <p className="t-caption">
                    {t("brief.updatedAt", {
                      when: formatDateTime(day.as_of, locale, viewerZone()),
                    })}
                  </p>
                </PanelBody>
                <PanelBody>
                  <BriefReadingsStrip day={day} />
                  <BriefCoverage
                    day={day}
                    onRetry={() => void query.refetch()}
                  />
                </PanelBody>
              </Panel>
            )}
            <BriefChanges />
            <SchedulePanel day={day} state={state} />
            <BriefUpdates day={day} />
            <OvernightPanel />
          </>
        )
      }
    />
  );
}
