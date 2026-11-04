// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The focus panel's body: the day's rows as cards, the one in hand opened.
// The panel's head, states and foot are in brief.feed.tsx.

import { useRef } from "react";
import {
  TaskNextSteps,
  TaskNextStepsPending,
} from "../../components/tasknextsteps";
import { Button, Card } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { useLocale, useT } from "../../i18n";
import { useActivity } from "../../lib/activityread";
import { navigate } from "../../lib/router";
import { useMe } from "../../lib/session";
import { briefContextTitle } from "../../lib/worklistcopy";
import type { WorklistItem } from "../worklist/worklist.queries";
import { WorklistRow } from "../worklist/worklist.row";
import { TaskCard, TaskMeta, TaskTick, taskName } from "./brief.taskcard";

import "./brief.feed.css";

/** One row's identity across renders, the way the queue page spells it. */
export function identity(item: WorklistItem): string {
  return `${item.source}-${item.id}`;
}

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
  // A press removes the pressed button from the list. Focus follows it onto
  // the card it brought in hand; a first render takes no focus.
  const pressed = useRef(false);
  const takeFocus = (frame: HTMLDivElement | null) => {
    if (frame && pressed.current) {
      pressed.current = false;
      frame.focus();
    }
  };
  // A task row's id is its activity's, so the card reads the task it shows.
  const isTask = lead.source === "task";
  const taskRead = useActivity(lead.id, isTask);
  const task = taskRead.data;
  const taskPending = isTask && taskRead.isPending;
  const cardVerbs =
    onContext || task || taskPending ? (
      <>
        {/* The drafted mail stands beside Done, ahead of the way to read
            more: it is the task's next step, the details are about it. */}
        {taskPending && <TaskNextStepsPending />}
        {task && <TaskNextSteps task={task} offerOpen={false} />}
        {/* Every row opens on what it asks and why it is here. */}
        {onContext && (
          <Button
            variant="ghost"
            className="brief-triage-details"
            onClick={() => onContext(lead)}
          >
            {t("brief.focus.context")}
          </Button>
        )}
      </>
    ) : undefined;
  // The open card says what to do, then its chips, then the deal's figures.
  // With no line of its own from the producer, the row's ask is its line.
  const told = Boolean(briefContextTitle(lead) || lead.detail);
  const inbox = lead.email_summary?.display_status;
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
              <Card
                as="article"
                className={
                  told
                    ? "brief-triage-lead"
                    : "brief-triage-lead brief-triage-lead-asks"
                }
              >
                <TaskTick item={lead} />
                <div className="brief-triage-body">
                  <div className="brief-triage-lead-head">
                    {lead.email_summary && (
                      <Heading size="small" as="h3">
                        {taskName(lead, viewer, t, locale)}
                      </Heading>
                    )}
                    <TaskMeta item={lead} linked />
                    {inbox && (
                      <p className="t-caption brief-triage-facts">
                        {t(`visibility.${inbox}`)}
                      </p>
                    )}
                  </div>
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
                    context={cardVerbs}
                    // The card around this row says whose it is, why it is
                    // here and where it stands in the day, each once.
                    framed
                  />
                </div>
              </Card>
            </div>
          </li>
        ) : (
          <li key={identity(item)}>
            <TaskCard
              item={item}
              onOpen={() => {
                pressed.current = true;
                onChoose(item);
              }}
            />
          </li>
        ),
      )}
    </ol>
  );
}
