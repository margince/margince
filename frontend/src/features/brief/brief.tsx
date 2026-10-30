// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useEffect, useId, useState } from "react";
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
import { markJourney } from "../../lib/journeymarks";
import { useMagic } from "../../lib/magic.queries";
import { isReviewableReceipt } from "../../lib/receiptreview";
import { navigate } from "../../lib/router";
import { useMe } from "../../lib/session";
import { TASK_PARAM } from "../../lib/taskreminders";
import { replaceDial, useUrlParams } from "../../lib/urlstate";
import { pillCount } from "../../lib/worklistcounts";
import { LinkedApprovalDrawer } from "../automations/approvaldrawer";
import { MagicPanel } from "../sign-in/magic";
import {
  MeetingDetailModal,
  TaskDetailModal,
} from "../worklist/taskdetailmodal";
import { drawerQueue } from "../worklist/worklist.address";
import { WorklistPane } from "../worklist/worklist.pane";
import {
  useHandledForYou,
  useWorklist,
  type WorklistItem,
  worklistKey,
} from "../worklist/worklist.queries";
import { readerTask } from "../worklist/worklist.reader";
import { changedSinceBrief } from "./brief.changed";
import { BriefChanges } from "./brief.changes";
import { BriefDials } from "./brief.dials";
import { briefDay } from "./brief.facts";
import { BriefFeed } from "./brief.feed";
import { BriefGlance } from "./brief.glance";
import { BriefIntro } from "./brief.intro";
import { homeJourneyMarks } from "./brief.journey";
import { PlanSection } from "./brief.plan";
import { useMorningBrief, useWeeklyReview } from "./brief.queries";
import { BriefQueue } from "./brief.queue";
import { BriefReadingsStrip } from "./brief.readings";
import { SchedulePanel } from "./brief.schedule";
import { BriefSuggestions } from "./brief.suggestions";
import { addressFrom, type BriefAddress, paramsFor } from "./brief.view";
import { WeeklySection, WeekStepper } from "./brief.weekly";
import { BriefCoverage } from "./briefcoverage";
import "./brief.css";

export function BriefScreen() {
  const t = useT();
  const nowMs = useNow(60_000);
  const me = useMe();
  const query = useWorklist("mine", "all");
  const [params, setParams] = useUrlParams();
  const address = addressFrom(params);
  const day = briefDay(query.data?.pages);
  // The queue the button opens, which keeps the drawer's scope and owner from
  // the address. It shares the drawer's query key, so the two report one total.
  const owner = params.get("owner") ?? "";
  const drawer = drawerQueue(params);
  const drawerOpen = params.get("queue") === "1";
  // The server counts every candidate before it applies a view, so Home's own
  // read already holds a closed drawer's total over the same queue.
  const homeQueue = drawer.scope === "mine" && owner === "";
  const queued = useWorklist(
    drawer.scope,
    "all",
    owner === "" ? undefined : owner,
    drawerOpen || !homeQueue,
    drawer.view,
  );
  const queuedDay =
    briefDay(queued.data?.pages) ?? (homeQueue ? day : undefined);
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
  // The way into the whole queue, with the day's count on it. The count is
  // the drawer's own total, read off the drawer's own query. It is the figure
  // the drawer's head sentence ends on, whichever queue it shows.
  const queueToggle = (
    <PageAsideToggle
      controlled={{
        open: drawerOpen,
        count: queuedDay ? pillCount(queuedDay, "all", drawer.view) : undefined,
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
  );
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
      briefedAt={morning ? brief.data?.generated_at : undefined}
      controls={dials}
      titleAside={morning ? undefined : <WeekStepper />}
    />
  );
  // The dial stands on the date's line, and the week stepper on the title's.
  const head = <div className="brief-head">{glance}</div>;
  return (
    <div className="wrap brief-wrap">
      {/* A link to one decision, from a notice or a receipt, opens it here. */}
      <LinkedApprovalDrawer />
      {morning && (
        <BriefIntro firstName={firstName} day={day} now={new Date(nowMs)} />
      )}
      <BriefBody
        address={address}
        query={query}
        head={head}
        queueToggle={queueToggle}
      />
      <BriefQueue />
    </div>
  );
}

export { deckItems } from "../../lib/brief.decisions.items";

function BriefBody({
  address,
  query,
  head,
  queueToggle,
}: Readonly<{
  address: BriefAddress;
  query: ReturnType<typeof useWorklist>;
  head: ReactNode;
  queueToggle: ReactNode;
}>) {
  if (address.view === "weekly")
    return (
      <div className="brief-weekly-page">
        {head}
        <WeeklySection plan={<PlanSection />} />
      </div>
    );
  return (
    <PersonalMorning query={query} head={head} queueToggle={queueToggle} />
  );
}

function PersonalMorning({
  query,
  head,
  queueToggle,
}: Readonly<{
  query: ReturnType<typeof useWorklist>;
  head: ReactNode;
  queueToggle: ReactNode;
}>) {
  const t = useT();
  const brief = useMorningBrief();
  const handled = useHandledForYou();
  const [selected, setSelected] = useState("");
  const [params] = useUrlParams();
  const update = useTaskUpdate([worklistKey]);
  const day = briefDay(query.data?.pages);
  const context = day?.focus?.items.find(
    (item) => `${item.source}-${item.id}` === selected,
  );
  // A task named by the address, as a reminder notification opens it. It
  // opens whether or not the day lists it, and writes only where a row the
  // reader holds offers `complete`.
  const linkedTask = context ? undefined : params.get(TASK_PARAM);
  const linkedRow = [
    ...(day?.focus?.items ?? []),
    ...(query.data?.pages.flatMap((page) => page.queue) ?? []),
  ].find((item) => item.source === "task" && item.id === linkedTask);
  const state = query.isPending
    ? "loading"
    : query.isError && !day
      ? "failed"
      : "ready";
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
              queueToggle={queueToggle}
            />
            <BriefSuggestions />
            <MagicPanel
              visibleApprovalIds={day?.focus?.items
                .filter(
                  (item) =>
                    item.source === "approval" &&
                    item.actions.includes("decide"),
                )
                .map((item) => item.id)}
              visibleReceiptIds={
                handled.isSuccess && !handled.isRefetchError
                  ? handled.data.receipts
                      ?.filter(isReviewableReceipt)
                      .map((receipt) => receipt.id)
                  : []
              }
            />
          </>
        }
        aside={
          <>
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
            <SchedulePanel day={day} state={state} />
            <BriefChanges />
          </>
        }
      />
      {linkedTask && (
        <TaskDetailModal
          activityId={linkedTask}
          item={linkedRow}
          readOnly={!linkedRow?.actions.includes("complete")}
          update={update}
          onClose={() => replaceDial(TASK_PARAM, undefined)}
        />
      )}
      {context && (
        <FocusDetails
          item={context}
          update={update}
          onClose={() => setSelected("")}
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
    <Modal open onClose={onClose} labelledBy={titleId} intent="drawer">
      <DrawerHead>
        <Heading size="large" id={titleId} className="t-h2">
          {t("brief.focus.context")}
        </Heading>
      </DrawerHead>
      <DrawerBody>
        <WorklistPane item={item} />
      </DrawerBody>
    </Modal>
  );
}
