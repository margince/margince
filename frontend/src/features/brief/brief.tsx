// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId, useState } from "react";
import { PageAsideToggle } from "../../components/pageaside";
import { Modal } from "../../design-system/atoms";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { Heading } from "../../design-system/heading";
import { PageZones } from "../../design-system/pagezones";
import { formatDateTime } from "../../format/format";
import { greetingNameOf } from "../../format/greetingname";
import { useNow } from "../../format/now";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { navigate } from "../../lib/router";
import { useMe } from "../../lib/session";
import { useUrlParams } from "../../lib/urlstate";
import { pillCount } from "../../lib/worklistcounts";
import { LinkedApprovalDrawer } from "../automations/approvaldrawer";
import { MagicPanel } from "../sign-in/magic";
import { useTaskUpdate } from "../worklist/taskactions";
import { TaskDetailModal } from "../worklist/taskdetailmodal";
import type { TeamWeekReach } from "../worklist/teamweekly.queries";
import { drawerScope } from "../worklist/worklist.address";
import { WorklistPane } from "../worklist/worklist.pane";
import { useWorklist, worklistKey } from "../worklist/worklist.queries";
import { readerTask } from "../worklist/worklist.reader";
import { changedSinceBrief } from "./brief.changed";
import { BriefChanges } from "./brief.changes";
import { BriefDials } from "./brief.dials";
import { briefDay } from "./brief.facts";
import { BriefFeed, BriefUpdates } from "./brief.feed";
import { BriefGlance } from "./brief.glance";
import { BriefIntro } from "./brief.intro";
import { PlanSection } from "./brief.plan";
import { useMorningBrief, useWeeklyReview } from "./brief.queries";
import { BriefQueue } from "./brief.queue";
import { OvernightDigest } from "./brief.rail.overnight";
import { BriefReadingsStrip } from "./brief.readings";
import { SchedulePanel } from "./brief.schedule";
import { BriefTeamBoard } from "./brief.teamboard";
import { BriefTeamSelect, morningTeams } from "./brief.teamselect";
import { TeamWeeklyPanel } from "./brief.teamweekly";
import {
  addressFrom,
  type BriefAddress,
  paramsFor,
  type TeamOffers,
} from "./brief.view";
import { WeeklySection } from "./brief.weekly";
import { BriefCoverage } from "./briefcoverage";
import "./brief.css";

export function BriefScreen() {
  const t = useT();
  const nowMs = useNow(60_000);
  const me = useMe();
  const own = useWorklist("mine", "all");
  // Each view's team surface on its own server rule: the team's live work on
  // row scope, the team's frozen week on who leads or oversees. An older
  // server without `team_week` offers no week, which is the safe reading.
  const teamWeek = own.data?.pages[0]?.team_week ?? "none";
  const teamOffered: TeamOffers = {
    morning: own.data?.pages[0]?.scope_options?.includes("team") ?? false,
    weekly: teamWeek !== "none",
  };
  const [params, setParams] = useUrlParams();
  const address = addressFrom(params, teamOffered);
  const query = own;
  const day = briefDay(query.data?.pages);
  // THE QUEUE THE BUTTON OPENS, which is not always the reader's own. The
  // drawer keeps its scope and its named owner in the address and both survive
  // it closing, so a reader who switched it to the team and shut it had a
  // button counting their own day over a list of somebody else's. The same
  // query key the drawer itself uses, so the two share one read and cannot
  // report two totals — and on the ordinary day, where neither is set, it IS
  // `own` and costs nothing.
  const owner = params.get("owner") ?? "";
  const queued = useWorklist(
    drawerScope(params),
    "all",
    owner === "" ? undefined : owner,
  );
  const queuedDay = briefDay(queued.data?.pages);
  const review = useWeeklyReview(address.week);
  const firstName = greetingNameOf(me.data?.user);
  const brief = useMorningBrief();
  const ownMorning = address.view === "morning" && address.scope === "mine";
  const glance = (
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
      briefedAt={ownMorning ? brief.data?.generated_at : undefined}
    />
  );
  const controls = (
    <div className="brief-controls">
      <BriefDials
        address={address}
        offered={teamOffered}
        onChange={(next) => setParams(paramsFor(next, params))}
      />
      {/* The way into the whole queue, as the head's PRIMARY control
        with the day's count on it: this is the one page whose pane is
        the day itself, so the switch is the main door and not a
        detail fold. The count is the DRAWER's own total, read off the
        drawer's own query — the same figure its head sentence ends
        on, whichever queue it is showing. */}
      <PageAsideToggle
        prominent
        controlled={{
          open: params.get("queue") === "1",
          count: queuedDay ? pillCount(queuedDay, "all") : undefined,
          labels: {
            show: t("brief.queue.show"),
            hide: t("brief.queue.hide"),
          },
          onToggle: () => {
            const next = new Map(params);
            if (next.get("queue") === "1") next.delete("queue");
            else next.set("queue", "1");
            navigate({ screen: "home" }, next);
          },
        }}
      />
    </div>
  );
  // Morning stacks the controls above the sentence, so they come first in
  // the DOM too: focus order follows the order the page is drawn in.
  const head = (
    <div className="brief-head">
      {ownMorning ? (
        <>
          {controls}
          {glance}
        </>
      ) : (
        <>
          {glance}
          {controls}
        </>
      )}
    </div>
  );
  return (
    <div className="wrap brief-wrap">
      {/* A link to one decision, from a notice or a receipt, opens it here. */}
      <LinkedApprovalDrawer />
      {ownMorning && (
        <BriefIntro firstName={firstName} day={day} now={new Date(nowMs)} />
      )}
      <BriefBody
        address={address}
        teamOffered={teamOffered}
        teamWeek={teamWeek}
        query={query}
        head={head}
      />
      <BriefQueue />
    </div>
  );
}

export { deckItems } from "./brief.decisions.items";

function BriefBody({
  address,
  teamOffered,
  teamWeek,
  query,
  head,
}: Readonly<{
  address: BriefAddress;
  teamOffered: TeamOffers;
  teamWeek: TeamWeekReach;
  query: ReturnType<typeof useWorklist>;
  head: ReactNode;
}>) {
  const me = useMe();
  if (address.view === "weekly")
    return (
      <>
        {head}
        {address.scope === "team" ? (
          <TeamWeeklyPanel reach={teamWeek} />
        ) : (
          <>
            <WeeklySection />
            <PlanSection />
          </>
        )}
      </>
    );
  if (address.scope === "team")
    return (
      <>
        {head}
        <BriefTeamSelect
          lists={morningTeams(me.data?.authorization?.row_scope)}
        >
          {(teamId) => (
            <BriefTeamBoard offered={teamOffered.morning} teamId={teamId} />
          )}
        </BriefTeamSelect>
      </>
    );
  return <PersonalMorning query={query} head={head} />;
}

function PersonalMorning({
  query,
  head,
}: Readonly<{ query: ReturnType<typeof useWorklist>; head: ReactNode }>) {
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
  // A task and a meeting are both ACTIVITIES, and the door on either row
  // opens the activity's own read: for a meeting, its subject and the
  // calendar's excerpt of who was there and what it was about. The pane
  // beside the queue draws only a contact, so a meeting sent there opened a
  // frame with nothing in it.
  const opensActivity =
    context?.source === "task" || context?.source === "meeting_outcome";
  return (
    <>
      {/* The brief and the work it names in one column; beside it, the
          figures and the meetings the work is read against. */}
      <PageZones
        shape="aside"
        className="brief-morning"
        mainClassName="brief-main"
        asideClassName="brief-rail"
        asideLabel={t("brief.rail")}
        main={
          <>
            {head}
            <BriefFeed
              day={day}
              onContext={(item) => setSelected(`${item.source}-${item.id}`)}
              state={state}
              changed={changedSinceBrief(day)}
              refreshFailed={query.isRefetchError}
              onRetry={() => void query.refetch()}
            />
            <BriefUpdates day={day} />
            <MagicPanel lead={<BriefChanges />} foot={<OvernightDigest />} />
          </>
        }
        aside={
          <>
            {day && (
              <div className="brief-overview">
                <BriefReadingsStrip day={day} />
                <BriefCoverage
                  day={day}
                  run={brief.data}
                  onRetry={() => void query.refetch()}
                />
                <p className="t-caption brief-freshness">
                  {t("brief.updatedAt", {
                    when: formatDateTime(day.as_of, locale, viewerZone()),
                  })}
                </p>
              </div>
            )}
            <SchedulePanel day={day} state={state} />
          </>
        }
      />
      {context && opensActivity ? (
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
          intent="drawer"
        >
          <DrawerHead>
            <Heading size="large" id={titleId} className="t-h2">
              {t("brief.focus.context")}
            </Heading>
          </DrawerHead>
          <DrawerBody>{context && <WorklistPane item={context} />}</DrawerBody>
        </Modal>
      )}
    </>
  );
}
