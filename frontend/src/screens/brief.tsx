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
import { MagicPanel } from "./magic";
import { TaskDetailModal, useTaskUpdate } from "./taskactions";
import { drawerScope } from "./worklist.address";
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
  const firstName = me.data?.user?.display_name?.trim().split(/\s+/)[0] ?? null;
  return (
    <div className="wrap brief-wrap">
      {/* THE HEAD ON THE PAGE GROUND, the way a record's head stands: the
          greeting where a record's name goes, the day over it, the sentence
          under it, and the controls on the far edge where a record's verbs
          stand. The one raised surface under it is the Focus panel — the
          work is the card, and everything around it is the page. */}
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
              count: queuedDay?.summary.total,
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
  // A task and a meeting are both ACTIVITIES, and the door on either row
  // opens the activity's own read: for a meeting, its subject and the
  // calendar's excerpt of who was there and what it was about. The pane
  // beside the queue draws only a contact, so a meeting sent there opened a
  // frame with nothing in it.
  const opensActivity =
    context?.source === "task" || context?.source === "meeting_outcome";
  return (
    <>
      {/* THE WORK, across the whole page: the Focus panel is the one card
          the morning is for, with the row in hand and the queue beside it,
          so it takes the page's width rather than sharing it with a rail. */}
      <BriefFeed
        day={day}
        onContext={(item) => setSelected(`${item.source}-${item.id}`)}
        state={state}
        changed={changedSinceBrief(day)}
        refreshFailed={query.isRefetchError}
        onRetry={() => void query.refetch()}
      />
      {/* THE FIGURES UNDER THE WORK: what the day holds in five readings,
          each a door into its lane. The work comes first because the page is
          for clearing it — the opening sentence already says what is first
          and how much follows — and the readings are what a reader turns to
          once the row in hand is answered. The band also carries what the
          read could not see and when it was assembled, because both qualify
          the figures. */}
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
      {/* THE FOLLOW-THROUGH under it, in two columns: what was done for the
          reader and the day's notices on the left, and on the right the
          context the work is read against — the schedule as the day's line,
          and the night. */}
      <PageZones
        shape="aside"
        className="brief-followthrough"
        mainClassName="record-stack brief-main"
        asideClassName="record-aside brief-rail"
        asideLabel={t("brief.rail")}
        main={
          <>
            <BriefChanges />
            <BriefUpdates day={day} />
            {/* THE RECEIPT, last and open. Everything above asks the reader for
                something; this asks for nothing, and it is the reason the acts
                above it are safe to take at all. It goes last because a reader
                opens this page to find what to do next and a list of what is
                already finished answers a different question. */}
            <MagicPanel />
          </>
        }
        aside={
          <>
            <SchedulePanel day={day} state={state} />
            <OvernightPanel />
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
