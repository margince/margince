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
import { Heading } from "../../design-system/heading";
import { PageZones } from "../../design-system/pagezones";
import { Panel, PanelBody } from "../../design-system/panel";
import { greetingNameOf } from "../../format/greetingname";
import { useNow } from "../../format/now";
import { useLocale, useT } from "../../i18n";
import { lazy } from "../../i18n/lazy";
import { APPROVAL_PARAM } from "../../lib/approvalhref";
import { useCan, useCanWrite } from "../../lib/capability";
import { markJourney } from "../../lib/journeymarks";
import { useMagic } from "../../lib/magic.queries";
import { usePendingMarginceChanges } from "../../lib/marginchanges";
import { useMe } from "../../lib/session";
import { TASK_PARAM } from "../../lib/taskreminders";
import { replaceDial, useUrlParams } from "../../lib/urlstate";
import { readerTask } from "../../lib/worklistcopy";
import { pillFigure } from "../../lib/worklistcounts";
import { DEFAULT_VIEW } from "../../lib/worklistviews";
import { MagicPanel, WAITING_LANES } from "../sign-in/magic";
import {
  useHandledForYou,
  useHomeDay,
  type useWorklist,
  type WorklistItem,
  worklistKey,
} from "../worklist/worklist.queries";
import { changedSinceBrief } from "./brief.changed";
import {
  ContactMeetingBrief,
  Quiet,
  WeeklyPage,
  WeekStepper,
  warmWhenIdle,
} from "./brief.deferred";
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
import { homeJourneyMarks } from "./brief.journey";
import { CustomerPaymentsCard } from "./brief.payments";
import { useMorningBrief, useWeeklyReview } from "./brief.queries";
import {
  BriefQueue,
  closeQueue,
  drawerOpen,
  loadWorklistScreen,
  openQueue,
} from "./brief.queue";
import { BriefReadingsPending, BriefReadingsStrip } from "./brief.readings";
import { SchedulePanel } from "./brief.schedule";
import { BriefSuggestions } from "./brief.suggestions";
import { taskName } from "./brief.taskcard";
import { shownTeam } from "./brief.teamselect";
import { addressFrom, type BriefAddress, paramsFor } from "./brief.view";
import { BriefCoverage } from "./briefcoverage";
import "./brief.css";

export function BriefScreen() {
  const t = useT();
  const nowMs = useNow(60_000);
  const me = useMe();
  const [params, setParams] = useUrlParams();
  const query = useHomeDay(Boolean(params.get(FOCUS_PARAM)));
  const asked = addressFrom(params);
  const address = { ...asked, team: shownTeam(asked.team, me.data?.team_week) };
  const day = briefDay(query.data?.pages);
  const queueOpen = drawerOpen(params);
  const review = useWeeklyReview(address.week, address.view === "weekly");
  const firstName = greetingNameOf(me.data?.user);
  const brief = useMorningBrief(useCan("deal", "read"));
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
        open: queueOpen,
        ...(day && pillFigure(day, "all", DEFAULT_VIEW)),
        labels: {
          show: t("brief.queue.show"),
          hide: t("brief.queue.hide"),
        },
        onToggle: queueOpen ? closeQueue : openQueue,
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
      // A team's week names itself below; the reader's own week is not its head.
      week={address.team ? undefined : review.data}
      firstName={firstName}
      now={new Date(nowMs)}
      pending={query.isPending}
      briefedAt={morning ? brief.data?.generated_at : undefined}
      controls={dials}
      titleAside={morning || address.team ? undefined : <WeekStepper />}
    />
  );
  return (
    <div className="wrap brief-wrap">
      {/* A link to one decision, from a notice or a receipt, opens it here. */}
      <LinkedApprovalDrawer />
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
  if (address.view === "weekly") return <WeeklyPage team={address.team} />;
  return <PersonalMorning query={query} queueToggle={queueToggle} />;
}

// Customer payments wait for the day's own reads, so their request never competes
// with the paint Home is measured on. A read that failed has answered too.
function dayAnswered(
  state: string,
  reads: readonly Readonly<{ isPending: boolean }>[],
): boolean {
  return state !== "loading" && reads.every((read) => !read.isPending);
}

// The deal reads never start for a reader without `deal.read`, so their
// sections are not drawn and nothing waits for them.
function useDealReads() {
  const readsDeals = useCan("deal", "read");
  const brief = useMorningBrief(readsDeals);
  const magic = useMagic(readsDeals);
  const pendingChanges = usePendingMarginceChanges(readsDeals);
  return {
    readsDeals,
    brief,
    magic,
    pendingChanges,
    dealReads: readsDeals ? [brief, magic] : [],
  };
}

function PersonalMorning({
  query,
  queueToggle,
}: Readonly<{
  query: ReturnType<typeof useWorklist>;
  queueToggle: ReactNode;
}>) {
  const t = useT();
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
  const { readsDeals, brief, magic, pendingChanges, dealReads } =
    useDealReads();
  const settled =
    handled.isSuccess && dealReads.every((read) => read.isSuccess);
  useEffect(() => {
    const marks = homeJourneyMarks({
      state,
      day,
      settled,
      receipt: magic.data,
    });
    for (const mark of marks) markJourney(mark);
    if (marks.includes("home:useful"))
      warmWhenIdle([loadDetails, loadPane, loadApproval, loadWorklistScreen]);
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
            {readsDeals && (
              <>
                <DoneForYou
                  completed={() => (
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
                  )}
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
            )}
          </>
        }
        aside={
          <>
            {/* The day's meetings first: the work is read against them. */}
            <ScheduleWithMeetings day={day} state={state} />
            {day && (
              <Panel
                title={t("brief.rail.numbers")}
                flatHead
                className="brief-overview"
              >
                <PanelBody>
                  <BriefReadingsStrip day={day} />
                  <BriefCoverage
                    day={day}
                    run={brief.data}
                    onRetry={() => void query.refetch()}
                  />
                </PanelBody>
              </Panel>
            )}
            {/* Hidden from a screen reader: the meetings above already
                announce the wait. */}
            {state === "loading" && (
              <div aria-hidden="true">
                <Panel
                  title={t("brief.rail.numbers")}
                  flatHead
                  className="brief-overview"
                >
                  <PanelBody>
                    <BriefReadingsPending />
                  </PanelBody>
                </Panel>
              </div>
            )}
            <CustomerPaymentsCard
              enabled={dayAnswered(state, [handled, ...dealReads])}
            />
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
  const { locale } = useLocale();
  const viewer = useMe(false).data?.user;
  const titleId = useId();
  if (item.source === "meeting_outcome") {
    return <MeetingDetailModal item={item} onClose={onClose} />;
  }
  if (item.source === "task") {
    return (
      <TaskDetailModal
        activityId={item.id}
        item={item}
        title={taskName(item, viewer, t, locale)}
        readOnly={!item.actions.includes("complete")}
        update={update}
        onClose={onClose}
      />
    );
  }
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="drawer" fit>
      <DrawerHead>
        {/* Named as the card names it, so the drawer says what it is about. */}
        <Heading size="large" id={titleId} className="t-h2">
          {taskName(item, viewer, t, locale)}
        </Heading>
      </DrawerHead>
      <DrawerBody className="form-stack">
        {/* The drawer covers the card, so it answers the row in place. */}
        <WorklistPane item={item} answer />
      </DrawerBody>
    </Modal>
  );
}

// The schedule, whose meeting titles open the meeting over Home and whose
// next meeting's brief opens there too.
function ScheduleWithMeetings({
  day,
  state,
}: Readonly<Omit<ComponentProps<typeof SchedulePanel>, "onOpenMeeting">>) {
  const [meeting, setMeeting] = useState<string | null>(null);
  const [prepFor, setPrepFor] = useState<string | null>(null);
  // Mounted from the first open on, so closing the brief plays its exit.
  const [briefAsked, setBriefAsked] = useState(false);
  return (
    <>
      <SchedulePanel
        day={day}
        state={state}
        onOpenMeeting={setMeeting}
        onPrepareMeeting={(activityId) => {
          setPrepFor(activityId);
          setBriefAsked(true);
        }}
      />
      {/* Opening it is what has Margince write the brief. */}
      {briefAsked && (
        <ContactMeetingBrief
          activityId={prepFor}
          open={prepFor !== null}
          onClose={() => setPrepFor(null)}
        />
      )}
      {meeting && (
        <ActivityDetailModal
          activityId={meeting}
          onClose={() => setMeeting(null)}
        />
      )}
    </>
  );
}

const loadDetails = () => import("../worklist/taskdetailmodal");
const loadPane = () => import("../worklist/worklist.pane");
const loadApproval = () => import("../automations/approvaldrawer");

const TaskDetailModal = lazy(
  async () => ({ default: (await loadDetails()).TaskDetailModal }),
  "complete",
  Quiet,
);

const MeetingDetailModal = lazy(
  async () => ({ default: (await loadDetails()).MeetingDetailModal }),
  "complete",
  Quiet,
);

const ActivityDetailModal = lazy(
  async () => ({ default: (await loadDetails()).ActivityDetailModal }),
  "complete",
  Quiet,
);

const WorklistPane = lazy(
  async () => ({ default: (await loadPane()).WorklistPane }),
  "complete",
  Quiet,
);

const ApprovalDrawer = lazy(
  async () => ({ default: (await loadApproval()).LinkedApprovalDrawer }),
  "complete",
  Quiet,
);

/**
 * A link to one decision, from a notice or a receipt, opens it over Home. Once
 * opened it stays mounted, so closing it plays the drawer's exit.
 */
function LinkedApprovalDrawer() {
  const [params] = useUrlParams();
  const [asked, setAsked] = useState(false);
  if (params.get(APPROVAL_PARAM) && !asked) setAsked(true);
  return asked ? <ApprovalDrawer /> : null;
}
