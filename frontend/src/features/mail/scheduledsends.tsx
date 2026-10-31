// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { AppLink } from "../../design-system/applink";
import {
  EmptyState,
  SectionHeader,
  SegmentedControl,
} from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { Disclosure } from "../../design-system/disclosure";
import { QueryStates, WriteRefused } from "../../design-system/querystates";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatNumber } from "../../format/format";
import { formatCountdown, useNow } from "../../format/now";
import { viewerZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import { useMailboxConnected } from "../../lib/connectors";
import { problemMessageOf } from "../../lib/problem";
import "./scheduledsends.css";
// The queue behind "send later", and the reason it has to exist: a rep who can
// schedule a message must be able to reach it again. Without this page the
// endpoints, the store and the fire path all existed and nothing in the client
// called them, so "send later" was a one-way door — the message a rep scheduled
// by mistake went out.
//
// It is the SENDER's own list, not the workspace's (ADR-0104/A155): an unsent
// body and its blind-copy list are not workspace-readable the way a sent
// activity is, and the server lists only what the caller scheduled. So there is
// no owner column here and no sharing affordance — the page is one contact's.
import { type ScheduledSend, useScheduledSends } from "./scheduledqueue";
import { TimelineDays } from "./scheduledsends.days";
import { EditComposer } from "./scheduledsends.edit";
import { QueueSkewNotice } from "./scheduledsends.notices";
import { SendRow } from "./scheduledsends.row";
import { useQueueWrites } from "./scheduledsends.writes";
import { dayGroups, readQueue } from "./scheduledtimeline";

/** The reader's cut through the queue; one at a time, and `all` shows the lot. */
const VIEWS = ["all", "held", "today"] as const;

type View = (typeof VIEWS)[number];

/** The countdown to the next message, with a clock of its own so the page above it stays still. */
function NextSend({ at }: Readonly<{ at: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const now = useNow(1000);
  const remaining = Date.parse(at) - now;
  if (remaining <= 0) return null;
  return (
    <p className="scheduledsends-next t-caption">
      {t("sched.next", { countdown: formatCountdown(remaining, t, locale) })}
    </p>
  );
}

/** The cut the reader chooses, with a figure on each, and the clock to the next send. */
function QueueToolbar({
  view,
  onView,
  counts,
  nextAt,
}: Readonly<{
  view: View;
  onView: (view: View) => void;
  counts: Readonly<{ waiting: number; held: number; today: number }>;
  nextAt: string | undefined;
}>) {
  const t = useT();
  return (
    <div className="scheduledsends-toolbar">
      <SegmentedControl
        label={t("sched.view")}
        options={VIEWS}
        value={view}
        onChange={onView}
        labels={{
          all: t("sched.view.all"),
          held: t("sched.view.held"),
          today: t("sched.view.today"),
        }}
        counts={{
          all: counts.waiting + counts.held,
          held: counts.held,
          today: counts.today,
        }}
        marks={{ held: counts.held > 0 }}
      />
      {nextAt && <NextSend at={nextAt} />}
    </div>
  );
}

// One line for the whole page. With a mailbox to send from it also offers the
// way to start, which is the contact the message goes to.
function EmptyQueue() {
  const t = useT();
  if (!useMailboxConnected()) {
    return <EmptyState>{t("sched.empty")}</EmptyState>;
  }
  return (
    <EmptyState
      title={t("sched.empty")}
      action={
        <AppLink className="btn btn-primary" href="#/contacts">
          {t("sched.write")}
        </AppLink>
      }
    >
      {t("sched.writeHow")}
    </EmptyState>
  );
}

export function ScheduledSendsScreen() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  // A moment a rep chose is a personal one, so the page reads in the viewer's
  // own zone unless the send names another (see Moment).
  const readerZone = viewerZone();
  const [withdrawing, setWithdrawing] = useState<ScheduledSend | null>(null);
  const [editing, setEditing] = useState<ScheduledSend | null>(null);
  const [view, setView] = useState<View>("all");
  // Re-read each minute so "today" and the presets stay true on a page left open.
  const now = new Date(useNow(60_000));

  const query = useScheduledSends();

  const { move, withdraw, moveWithUndo, skew, writeError } = useQueueWrites(
    readerZone,
    () => setWithdrawing(null),
  );

  const { held, waiting, closed, goingToday, next, counts } = readQueue(
    query.data ?? [],
    now,
    readerZone,
  );
  const days = dayGroups(view === "today" ? goingToday : waiting, readerZone);
  const nextId = next?.id;
  const nextAt = next?.scheduled_at;

  const row = (send: ScheduledSend, timeOnly: boolean) => (
    <SendRow
      key={send.id}
      send={send}
      readerZone={readerZone}
      now={now}
      timeOnly={timeOnly}
      isNext={send.id === nextId}
      movePending={move.isPending}
      onMove={(input) => moveWithUndo(input, send)}
      onEdit={setEditing}
      onWithdraw={setWithdrawing}
    />
  );

  return (
    // No h1 of its own: the shell's PageTitle names this page and prints its
    // subtitle, and a second h1 would leave two page titles.
    <div className="wrap">
      {skew && (
        <QueueSkewNotice message={skew} onReload={() => void query.refetch()} />
      )}
      <WriteRefused titleKey="sched.writeFailed" message={writeError} />
      <QueryStates query={query} pendingLabel={t("nav.scheduled")}>
        {query.data && query.data.length === 0 ? (
          <EmptyQueue />
        ) : (
          query.data && (
            <div className="arrive-stack">
              <QueueToolbar
                view={view}
                onView={setView}
                counts={{ ...counts, today: goingToday.length }}
                nextAt={nextAt}
              />
              {view !== "today" && held.length > 0 && (
                <section aria-label={t("sched.group.held")}>
                  <SectionHeader title={t("sched.group.held")} />
                  <ol className="scheduledsends-list">
                    {held.map((send) => row(send, false))}
                  </ol>
                </section>
              )}
              {view !== "held" && (
                <section aria-label={t("sched.group.waiting")}>
                  <SectionHeader title={t("sched.group.waiting")} />
                  <SurfaceState
                    loadingLabel={t("sched.group.waiting")}
                    state={days.length === 0 ? "empty" : "ready"}
                    emptyLabel={t(
                      view === "today"
                        ? "sched.todayEmpty"
                        : "sched.group.waitingEmpty",
                    )}
                  >
                    <TimelineDays
                      days={days}
                      now={now}
                      zone={readerZone}
                      renderRow={(send) => row(send, true)}
                    />
                  </SurfaceState>
                </section>
              )}
              {view === "all" && closed.length > 0 && (
                <Disclosure
                  summary={plural("sched.group.closedCount", closed.length, {
                    count: formatNumber(closed.length, locale),
                  })}
                >
                  <ol className="scheduledsends-list">
                    {closed.map((send) => row(send, false))}
                  </ol>
                </Disclosure>
              )}
            </div>
          )
        )}
      </QueryStates>
      <EditComposer
        send={editing}
        onClose={() => setEditing(null)}
        onReplaced={(id) => withdraw.mutate({ id })}
      />
      {/* Withdrawing is confirmed and moving is not, and the difference is what
          each one costs to undo: a moved message can be moved back, while a
          withdrawn one is gone and has to be written again from nothing — the
          approval it carried does not survive it. */}
      <ConfirmModal
        open={withdrawing !== null}
        onClose={() => setWithdrawing(null)}
        title={t("sched.withdrawTitle")}
        confirmLabel={t("sched.withdrawConfirm")}
        confirmVariant="danger"
        pending={withdraw.isPending}
        error={withdraw.isError ? problemMessageOf(withdraw.error, t) : null}
        onConfirm={() => {
          if (withdrawing) {
            withdraw.mutate({ id: withdrawing.id });
          }
        }}
      >
        <p>
          {t("sched.withdrawBody", { subject: withdrawing?.subject ?? "" })}
        </p>
      </ConfirmModal>
    </div>
  );
}
