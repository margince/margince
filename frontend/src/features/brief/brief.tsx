// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type ComponentProps,
  type ReactNode,
  useEffect,
  useId,
  useState,
} from "react";
import { PageAsideToggle } from "../../components/pageaside";
import { useTaskUpdate } from "../../components/taskactions";
import { Modal } from "../../design-system/atoms";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { Eyebrow } from "../../design-system/eyebrow";
import { Heading } from "../../design-system/heading";
import { PageZones } from "../../design-system/pagezones";
import { greetingNameOf } from "../../format/greetingname";
import { useNow } from "../../format/now";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { markJourney } from "../../lib/journeymarks";
import { useMagic } from "../../lib/magic.queries";
import { usePendingMarginceChanges } from "../../lib/marginchanges";
import { navigate } from "../../lib/router";
import { useMe } from "../../lib/session";
import { TASK_PARAM } from "../../lib/taskreminders";
import { replaceDial, useUrlParams } from "../../lib/urlstate";
import { readerTask } from "../../lib/worklistcopy";
import { pillFigure } from "../../lib/worklistcounts";
import { WORKLIST_VIEW_PARAM } from "../../lib/worklistrouting";
import { DEFAULT_VIEW } from "../../lib/worklistviews";
import { LinkedApprovalDrawer } from "../automations/approvaldrawer";
import { MagicPanel, WAITING_LANES } from "../sign-in/magic";
import {
  ActivityDetailModal,
  MeetingDetailModal,
  TaskDetailModal,
} from "../worklist/taskdetailmodal";
import { WorklistPane } from "../worklist/worklist.pane";
import {
  useHandledForYou,
  useWorklist,
  type WorklistItem,
  worklistKey,
} from "../worklist/worklist.queries";
import { changedSinceBrief } from "./brief.changed";
import { BriefDials } from "./brief.dials";
import { DoneForYou } from "./brief.doneforyou";
import { shownReceiptIds } from "./brief.doneforyou.groups";
import { briefDay } from "./brief.facts";
import { BriefFeed } from "./brief.feed";
import {
  dayRows,
  FOCUS_PARAM,
  identity,
  rowByKey,
  useStaleFocusCleared,
} from "./brief.focuslink";
import { BriefGlance } from "./brief.glance";
import { BriefIntro } from "./brief.intro";
import { homeJourneyMarks } from "./brief.journey";
import { PlanSection } from "./brief.plan";
import { PlanConstraints } from "./brief.plan.contract";
import { useMorningBrief, useWeeklyReview } from "./brief.queries";
import { BriefQueue } from "./brief.queue";
import { BriefReadingsPending, BriefReadingsStrip } from "./brief.readings";
import { SchedulePanel } from "./brief.schedule";
import { BriefSuggestions } from "./brief.suggestions";
import { addressFrom, type BriefAddress, paramsFor } from "./brief.view";
import { WeeklySection, WeekStepper } from "./brief.weekly";
import { useWeeklyRailShown, WeeklyRail } from "./brief.weekly.rail";
import { BriefCoverage } from "./briefcoverage";
import "./brief.css";

// The dials that narrow the drawer. The Home button clears them, so it always
// opens the reader's own Today with every category.
const DRAWER_DIALS = [
  "queue_scope",
  WORKLIST_VIEW_PARAM,
  "filter",
  "owner",
  "selected",
];

export function BriefScreen() {
  const t = useT();
  const nowMs = useNow(60_000);
  const me = useMe();
  // The drawer's default queue, so an open drawer joins this read.
  const query = useWorklist("mine", "all", undefined, true, DEFAULT_VIEW);
  const [params, setParams] = useUrlParams();
  const address = addressFrom(params);
  const day = briefDay(query.data?.pages);
  const drawerOpen = params.get("queue") === "1";
  const review = useWeeklyReview(address.week, address.view === "weekly");
  const firstName = greetingNameOf(me.data?.user);
  const brief = useMorningBrief();
  const morning = address.view === "morning";
  const dials = (
    <BriefDials
      address={address}
      onChange={(next) => setParams(paramsFor(next, params))}
    />
  );
  // The way into the reader's own Today, with that list's count on it. It
  // always opens Mine, Today, every category, whatever view the drawer was
  // last left on, so the number is the one the opened list shows.
  const queueToggle = (
    <PageAsideToggle
      controlled={{
        open: drawerOpen,
        ...(day && pillFigure(day, "all", DEFAULT_VIEW)),
        labels: {
          show: t("brief.queue.show"),
          hide: t("brief.queue.hide"),
        },
        onToggle: () => {
          const next = new Map(params);
          if (next.get("queue") === "1") {
            next.delete("queue");
          } else {
            for (const dial of DRAWER_DIALS) next.delete(dial);
            next.set("queue", "1");
          }
          navigate({ screen: "home" }, next);
        },
      }}
    />
  );
  // The dial ends the facts' line, and the week stepper the title's.
  const glance = (
    <BriefGlance
      view={address.view}
      day={
        day
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
      week={review.data}
      firstName={firstName}
      now={new Date(nowMs)}
      pending={query.isPending}
      briefedAt={morning ? brief.data?.generated_at : undefined}
      controls={dials}
      titleAside={morning ? undefined : <WeekStepper />}
    />
  );
  return (
    <div className="wrap brief-wrap">
      {/* A link to one decision, from a notice or a receipt, opens it here. */}
      <LinkedApprovalDrawer />
      {morning && (
        <BriefIntro firstName={firstName} day={day} now={new Date(nowMs)} />
      )}
      {/* The head stands over the panes in both views, so the dial never
          moves when it is pressed. */}
      <div className="brief-frame">
        {glance}
        <BriefBody address={address} query={query} queueToggle={queueToggle} />
      </div>
      <BriefQueue />
    </div>
  );
}

export { deckItems } from "../../lib/brief.decisions.items";

function BriefBody({
  address,
  query,
  queueToggle,
}: Readonly<{
  address: BriefAddress;
  query: ReturnType<typeof useWorklist>;
  queueToggle: ReactNode;
}>) {
  if (address.view === "weekly") return <WeeklyPage />;
  return <PersonalMorning query={query} queueToggle={queueToggle} />;
}

function WeeklyPage() {
  const t = useT();
  // An aside handed over empty still draws its column and landmark.
  const railShown = useWeeklyRailShown();
  return (
    <PageZones
      shape="aside"
      panes
      className="brief-weekly-page"
      mainClassName="brief-main"
      asideClassName="brief-rail"
      asideLabel={t("brief.weekly.rail")}
      main={<WeeklySection plan={<PlanSection withContract={false} />} />}
      aside={
        railShown ? <WeeklyRail constraints={<PlanConstraints />} /> : undefined
      }
    />
  );
}

function PersonalMorning({
  query,
  queueToggle,
}: Readonly<{
  query: ReturnType<typeof useWorklist>;
  queueToggle: ReactNode;
}>) {
  const t = useT();
  const brief = useMorningBrief();
  const handled = useHandledForYou();
  const [selected, setSelected] = useState("");
  const [params] = useUrlParams();
  // Finishing a task also changes the advice on its deal and contact.
  const update = useTaskUpdate([worklistKey, ["deal-status"], ["contact360"]]);
  // A linked task may sit beyond the loaded queue; its own read says whether
  // this reader may change it, as the Worklist drawer asks.
  const canUpdateTask = useCanWrite("activity", "update");
  const day = briefDay(query.data?.pages);
  const rows = dayRows(day, query.data?.pages);
  // A row named by the address opens as a click on it does; mail links there.
  const linkedFocus = params.get(FOCUS_PARAM);
  const context = rowByKey(rows, linkedFocus || selected);
  useStaleFocusCleared(
    linkedFocus,
    context,
    query.isSuccess && !query.isFetching,
  );
  // A task named by the address, as a reminder notification opens it. It
  // opens whether or not the day lists it, and writes only where a row the
  // reader holds offers `complete`.
  const linkedTask = context ? undefined : params.get(TASK_PARAM);
  const linkedRow = linkedTask
    ? rowByKey(rows, identity({ source: "task", id: linkedTask }))
    : undefined;
  const state = query.isPending
    ? "loading"
    : query.isError && !day
      ? "failed"
      : "ready";
  const pendingChanges = usePendingMarginceChanges();
  const magic = useMagic();
  const settled = brief.isSuccess && handled.isSuccess && magic.isSuccess;
  useEffect(() => {
    const marks = homeJourneyMarks({
      state,
      day,
      settled,
      receipt: magic.data,
    });
    for (const mark of marks) markJourney(mark);
  }, [state, day, settled, magic.data]);
  return (
    <>
      {/* The work in one column; beside it, the meetings and the figures
          the work is read against. */}
      <PageZones
        shape="aside"
        panes
        className="brief-morning"
        mainClassName="brief-main"
        asideClassName="brief-rail"
        asideLabel={t("brief.rail")}
        main={
          <>
            <BriefFeed
              day={day}
              onContext={(item) => setSelected(identity(item))}
              state={state}
              changed={changedSinceBrief(day)}
              refreshFailed={query.isRefetchError}
              onRetry={() => void query.refetch()}
              queueToggle={queueToggle}
            />
            {/* What Margince finished stands with the work, where a reader
                reviews or undoes it, not in the rail beside it. */}
            <DoneForYou
              completed={
                <MagicPanel
                  lanes={["done"]}
                  embedded
                  visibleReceiptIds={
                    handled.isSuccess && !handled.isRefetchError
                      ? shownReceiptIds(
                          handled.data.receipts,
                          pendingChanges.data?.bundles,
                        )
                      : []
                  }
                />
              }
            />
            <BriefSuggestions />
            <MagicPanel
              lanes={WAITING_LANES}
              visibleApprovalIds={day?.focus?.items
                .filter(
                  (item) =>
                    item.source === "approval" &&
                    item.actions.includes("decide"),
                )
                .map((item) => item.id)}
            />
          </>
        }
        aside={
          <>
            {/* The day's meetings first: the work is read against them. */}
            <ScheduleWithMeetings day={day} state={state} />
            {day && (
              <div className="brief-overview">
                <Eyebrow as="h2" className="brief-section-label">
                  {t("brief.rail.numbers")}
                </Eyebrow>
                <BriefReadingsStrip day={day} />
                <BriefCoverage
                  day={day}
                  run={brief.data}
                  onRetry={() => void query.refetch()}
                />
              </div>
            )}
            {/* Hidden from a screen reader: the meetings above already
                announce the wait. */}
            {state === "loading" && (
              <div className="brief-overview" aria-hidden="true">
                <Eyebrow as="h2" className="brief-section-label">
                  {t("brief.rail.numbers")}
                </Eyebrow>
                <BriefReadingsPending />
              </div>
            )}
          </>
        }
      />
      {linkedTask && (
        <TaskDetailModal
          activityId={linkedTask}
          item={linkedRow}
          readOnly={!canUpdateTask}
          update={update}
          onClose={() => replaceDial(TASK_PARAM, undefined)}
        />
      )}
      {context && (
        <FocusDetails
          item={context}
          update={update}
          onClose={() => {
            setSelected("");
            if (linkedFocus) replaceDial(FOCUS_PARAM, undefined);
          }}
        />
      )}
    </>
  );
}

// The row in hand, opened. A task opens its own read, a meeting outcome the
// meeting reader, and any other row what it asks beside its contact.
function FocusDetails({
  item,
  update,
  onClose,
}: Readonly<{
  item: WorklistItem;
  update: ReturnType<typeof useTaskUpdate>;
  onClose: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  if (item.source === "meeting_outcome") {
    return <MeetingDetailModal item={item} onClose={onClose} />;
  }
  if (item.source === "task") {
    return (
      <TaskDetailModal
        activityId={item.id}
        item={item}
        readOnly={!item.actions.includes("complete")}
        update={update}
        onClose={onClose}
      />
    );
  }
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="drawer" fit>
      <DrawerHead>
        <Heading size="large" id={titleId} className="t-h2">
          {t("brief.focus.context")}
        </Heading>
      </DrawerHead>
      <DrawerBody className="form-stack">
        {/* The drawer covers the card, so it answers the row in place. */}
        <WorklistPane item={item} answer />
      </DrawerBody>
    </Modal>
  );
}

// The schedule, whose meeting titles open the meeting over Home.
function ScheduleWithMeetings({
  day,
  state,
}: Readonly<Omit<ComponentProps<typeof SchedulePanel>, "onOpenMeeting">>) {
  const [meeting, setMeeting] = useState<string | null>(null);
  return (
    <>
      <SchedulePanel day={day} state={state} onOpenMeeting={setMeeting} />
      {meeting && (
        <ActivityDetailModal
          activityId={meeting}
          onClose={() => setMeeting(null)}
        />
      )}
    </>
  );
}
