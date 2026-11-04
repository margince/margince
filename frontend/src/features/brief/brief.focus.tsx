// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The focus panel's body: the day's rows as cards, the one in hand opened.
// The panel's head, states and foot are in brief.feed.tsx.

import { useRef } from "react";
import {
  TaskNextSteps,
  TaskNextStepsPending,
} from "../../components/tasknextsteps";
import { AppLink } from "../../design-system/applink";
import { Button, Card } from "../../design-system/atoms";
import { activityDay, activityTimeOfDay } from "../../format/activitywhen";
import { formatDayMonth } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { useActivity } from "../../lib/activityread";
import { useRecordZone } from "../../lib/recordzone";
import { navigate } from "../../lib/router";
import { briefSubjectLabel, subjectHref } from "../../lib/worklistcopy";
import { eyebrowKeyFor } from "../../lib/worklisteyebrow";
import type { WorklistItem } from "../worklist/worklist.queries";
import { WorklistRow } from "../worklist/worklist.row";
import { contactHref, useListed } from "../worklist/worklist.row.captions";
import { cardWhen, TaskCard, TaskTick } from "./brief.taskcard";

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
  const zone = viewerZone();
  const recordZone = useRecordZone();
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
          <Button variant="ghost" onClick={() => onContext(lead)}>
            {t("brief.focus.context")}
          </Button>
        )}
      </>
    ) : undefined;
  // The head's facts come from the helpers the row uses, so the card and the
  // queue agree. A message is dated by its arrival, the rest by its own clock.
  // The reader's own pin is the queue's reason, not the card's.
  const arrived = lead.email_summary;
  const when = arrived
    ? [
        activityDay(
          arrived.occurred_at,
          arrived.occurred_precision,
          locale,
          zone,
          formatDayMonth,
        ),
        activityTimeOfDay(
          arrived.occurred_at,
          arrived.occurred_precision,
          locale,
          zone,
        ),
      ]
        .filter(Boolean)
        .join(", ")
    : cardWhen(lead, t, locale, zone, recordZone);
  const label = t(eyebrowKeyFor(lead));
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
              <Card as="article" className="brief-triage-lead">
                <TaskTick item={lead} />
                <div className="brief-triage-body">
                  <p className="t-caption brief-triage-lead-head">
                    <span
                      className={
                        lead.band === "now"
                          ? "brief-triage-label brief-triage-label-now"
                          : "brief-triage-label"
                      }
                    >
                      {label}
                    </span>
                    <AboutLine item={lead} />
                    {when && <span className="t-num">{when}</span>}
                  </p>
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

/**
 * Whose row this is, linked: the contact the server put on the row, else the
 * record it is about. The sender of a thread filed under a deal is a contact,
 * and the reply goes to them. Where the message sits (the team's inbox, the
 * reader's own) follows in the access badge's own words.
 */
function AboutLine({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const contact = item.contact;
  const label =
    contact?.label ??
    (briefSubjectLabel(item) ? undefined : item.subject?.label);
  const listed = useListed();
  const target = contact?.label ? contactHref(contact) : subjectHref(item);
  const href = target && listed(target);
  const inbox = item.email_summary?.display_status;
  return (
    <>
      {label && href && (
        <AppLink className="entity-link" href={href}>
          {label}
        </AppLink>
      )}
      {inbox && <span>{t(`visibility.${inbox}`)}</span>}
    </>
  );
}
