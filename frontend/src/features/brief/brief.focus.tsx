// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The focus panel's body: the day's rows as cards, the one in hand opened.
// The panel's head, states and foot are in brief.feed.tsx.

import { ArrowRight } from "lucide-react";
import { useRef } from "react";
import { TaskMeta } from "../../components/taskmeta";
import {
  TaskNextStepsPending,
  useTaskNextSteps,
} from "../../components/tasknextsteps";
import { Button, Card } from "../../design-system/atoms";
import { useFocusHandoff } from "../../design-system/focushandoff";
import { Heading } from "../../design-system/heading";
import { useLocale, useT } from "../../i18n";
import { useActivity } from "../../lib/activityread";
import { navigate } from "../../lib/router";
import { useMe } from "../../lib/session";
import { taskTypeOf } from "../../lib/tasktype";
import { briefContextTitle } from "../../lib/worklistcopy";
import { emailNamesRow } from "../../lib/worklistemailrow";
import type { WorklistItem } from "../worklist/worklist.queries";
import { OneRowWrite, useRowDone, WorklistRow } from "../worklist/worklist.row";
import { useListed } from "../worklist/worklist.row.captions";
import { TaskCard, TaskTick, taskName } from "./brief.taskcard";

import "./brief.feed.css";
import { identity } from "./brief.focuslink";

export function Triage({
  rows,
  at,
  lead,
  onChoose,
  onOpenEmail,
  onContext,
}: Readonly<{
  rows: readonly WorklistItem[];
  at: number;
  lead: WorklistItem;
  onChoose: (item: WorklistItem) => void;
  onOpenEmail: (id: string) => void;
  onContext?: (item: WorklistItem) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const viewer = useMe(false).data?.user;
  const listed = useListed();
  // A press removes the pressed button, and a finished task takes its card
  // away. Either way focus moves onto the card now in hand. A first render
  // takes no focus.
  const pressed = useRef(false);
  const held = useRef<string | null>(null);
  const takeFocus = (frame: HTMLDivElement | null) => {
    if (!frame) return;
    const key = identity(lead);
    const replaced = held.current !== null && held.current !== key;
    held.current = key;
    if (
      pressed.current ||
      (replaced && document.activeElement === document.body)
    ) {
      pressed.current = false;
      frame.focus();
    }
  };
  // A task row's id is its activity's, so the card reads the task it shows.
  const isTask = lead.source === "task";
  const taskRead = useActivity(lead.id, isTask);
  const task = taskRead.data;
  const taskPending = isTask && taskRead.isPending;
  // The type's own step is the card's one filled verb, so Done steps back.
  // A refused draft still draws its AI button, with the reason under it.
  const step = useTaskNextSteps(task, false);
  // Next steps past a task without answering it; past the last it wraps.
  // It sits at the title's end, apart from the verbs that answer the task.
  const next = rows.length > 1 ? rows[(at + 1) % rows.length] : undefined;
  const nextLink = next && (
    <Button
      variant="link"
      className="brief-triage-next"
      onClick={() => {
        pressed.current = true;
        onChoose(next);
      }}
    >
      {t("brief.focus.next")} <ArrowRight aria-hidden />
    </Button>
  );
  // Every row opens on what it asks and why it is here, first on the line.
  const details = onContext && (
    <Button
      variant="ghost"
      className="brief-triage-details"
      onClick={() => onContext(lead)}
    >
      {t("brief.focus.context")}
    </Button>
  );
  // The type's step stands beside Done: it is the task's next step.
  const cardVerbs =
    task || taskPending ? (
      <>
        {taskPending && <TaskNextStepsPending />}
        {step.steps}
      </>
    ) : undefined;
  // The open card says what to do, then its chips, then the deal's figures.
  // With no line of its own from the producer, the row's ask is its line.
  const told = Boolean(briefContextTitle(lead) || lead.detail);
  const inbox = lead.email_summary?.display_status;
  // The chips are read after the name, so they follow it in the page too. They
  // sit in the head after a message's own title, or else under the row's title.
  const head = (
    <div className="brief-triage-lead-head">
      {emailNamesRow(lead) && (
        <div className="brief-triage-title">
          <Heading size="small" as="h3">
            {taskName(lead, viewer, t, locale)}
          </Heading>
          {nextLink}
        </div>
      )}
      <TaskMeta item={lead} link={listed} taskType={task && taskTypeOf(task)} />
      {inbox && (
        <p className="t-caption brief-triage-facts">
          {t(`visibility.${inbox}`)}
        </p>
      )}
    </div>
  );
  // Every row of the day is a card in the server's order, and the one in hand
  // opens where it stands. An ordered list, so the order reaches a screen
  // reader as the claim it is.
  return (
    <ol className="brief-triage">
      {rows.map((item, index) =>
        index === at ? (
          <li key={identity(item)}>
            {/* Keyed by the row, so pressing another one mounts a new card and
                replays its arrival. The press is answered on the card. */}
            <div ref={takeFocus} tabIndex={-1} className="brief-triage-frame">
              <OneRowWrite item={lead}>
                <Card
                  as="article"
                  className={
                    told
                      ? "brief-triage-lead"
                      : "brief-triage-lead brief-triage-lead-asks"
                  }
                >
                  <LeadTick item={lead} />
                  <div className="brief-triage-body">
                    {emailNamesRow(lead) && head}
                    <WorklistRow
                      // Personal ordering is the queue's, not the focus projection's.
                      allowPin={false}
                      item={lead}
                      position={at + 1}
                      owner=""
                      onOpenEmail={onOpenEmail}
                      onReview={() =>
                        navigate(
                          { screen: "worklist" },
                          new Map([["filter", lead.category]]),
                        )
                      }
                      context={details}
                      verbs={cardVerbs}
                      menu={step.others}
                      titleEnd={emailNamesRow(lead) ? undefined : nextLink}
                      underTitle={emailNamesRow(lead) ? undefined : head}
                      quiet={step.fills}
                      // The card around this row says whose it is, why it is
                      // here and where it stands in the day, each once.
                      framed
                    />
                  </div>
                </Card>
              </OneRowWrite>
            </div>
          </li>
        ) : (
          <QueuedCard
            key={identity(item)}
            item={item}
            onOpen={() => {
              pressed.current = true;
              onChoose(item);
            }}
          />
        ),
      )}
    </ol>
  );
}

/** A card waiting behind the one in hand, with Done on its tick where offered. */
function QueuedCard({
  item,
  onOpen,
}: Readonly<{ item: WorklistItem; onOpen: () => void }>) {
  // Done from its own tick, the card leaves holding focus. The card that takes
  // its place takes it, or the one before it at the end.
  const card = useRef<HTMLLIElement>(null);
  useFocusHandoff(card, () => {
    const after = card.current?.nextElementSibling;
    const before = card.current?.previousElementSibling;
    return (
      (after ?? before)?.querySelector<HTMLElement>(
        ".brief-triage-frame, .brief-task-open",
      ) ?? null
    );
  });
  return (
    <li ref={card}>
      <TaskCard item={item} onOpen={onOpen} rowDone={useRowDone(item)} />
    </li>
  );
}

/** The tick on the card in hand, sharing the one write its row's verbs use. */
function LeadTick({ item }: Readonly<{ item: WorklistItem }>) {
  return <TaskTick item={item} rowDone={useRowDone(item)} />;
}
