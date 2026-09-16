// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { PageAsideToggle } from "../app/pageaside";
import { navigate } from "../app/router";
import { useUrlParams } from "../app/urlstate";
import { Button, Modal } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { PageZones } from "../design-system/pagezones";
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
import { TaskDetailModal, useTaskUpdate } from "./taskactions";
import { WorklistPane } from "./worklist.pane";
import { useWorklist, worklistKey } from "./worklist.queries";
import { readerTask } from "./worklist.reader";
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
          day={
            address.scope === "mine" && day
              ? {
                  ...day,
                  focus: day.focus
                    ? {
                        ...day.focus,
                        items: day.focus.items.map((item) =>
                          readerTask(item, me.data?.user, t),
                        ),
                      }
                    : undefined,
                }
              : undefined
          }
          week={address.scope === "mine" ? review.data : undefined}
          firstName={firstName}
          now={new Date(nowMs)}
        />
        <div className="brief-controls">
          <BriefDials
            address={address}
            offered={teamOffered}
            onChange={(next) => setParams(paramsFor(next, params))}
          />
          <PageAsideToggle
            controlled={{
              open: params.get("queue") === "1",
              label: t("brief.queue.title"),
              onToggle: () => {
                const next = new Map(params);
                if (next.get("queue") === "1") next.delete("queue");
                else next.set("queue", "1");
                navigate({ screen: "home" }, next);
              },
            }}
          />
        </div>
      </div>
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
  const titleId = useId();
  const update = useTaskUpdate([worklistKey]);
  const day = briefDay(query.data?.pages);
  const context = day?.focus?.items.find(
    (item) => `${item.source}-${item.id}` === selected,
  );
  const state = query.isPending
    ? "loading"
    : query.isError && !day
      ? "failed"
      : "ready";
  return (
    <>
      <BriefFeed
        day={day}
        onContext={(item) => setSelected(`${item.source}-${item.id}`)}
        state={state}
        changed={changedSinceBrief(day)}
        refreshFailed={query.isRefetchError}
        onRetry={() => void query.refetch()}
      />
      {day && (
        <div className="brief-overview">
          <BriefReadingsStrip day={day} />
          <BriefCoverage day={day} onRetry={() => void query.refetch()} />
          <p className="t-caption brief-freshness">
            {t("brief.updatedAt", {
              when: formatDateTime(day.as_of, locale, viewerZone()),
            })}
            {brief.data?.generated_at && (
              <>
                {" "}
                ·{" "}
                {t("brief.createdAt", {
                  when: formatDateTime(
                    brief.data.generated_at,
                    locale,
                    viewerZone(),
                  ),
                })}
              </>
            )}
          </p>
        </div>
      )}
      <PageZones
        shape="aside"
        className="brief-followthrough"
        mainClassName="brief-main"
        asideClassName="brief-rail"
        asideLabel={t("brief.rail")}
        main={
          <>
            <BriefChanges />
            <BriefUpdates day={day} />
          </>
        }
        aside={
          <>
            <SchedulePanel day={day} state={state} />
            <OvernightPanel />
          </>
        }
      />
      {context?.source === "task" ? (
        <TaskDetailModal
          activityId={context.id}
          readOnly={!context.actions.includes("complete")}
          update={update}
          onClose={() => setSelected("")}
        />
      ) : (
        <Modal
          open={Boolean(context)}
          onClose={() => setSelected("")}
          labelledBy={titleId}
          placement="right"
        >
          <div className="drawer-head brief-queue-head">
            <Heading size="large" id={titleId} className="t-h2">
              {t("brief.focus.context")}
            </Heading>
            <Button variant="ghost" onClick={() => setSelected("")}>
              {t("brief.focus.back")}
            </Button>
          </div>
          <div className="drawer-body">
            {context && <WorklistPane item={context} />}
          </div>
        </Modal>
      )}
    </>
  );
}
