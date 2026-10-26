// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The focus panel's body: the row in hand as a card, and the ranked column
// beside it. The panel's head, states and foot are in brief.feed.tsx.

import { useRef } from "react";
import { TaskNextSteps } from "../../components/tasknextsteps";
import { Button, Card } from "../../design-system/atoms";
import { Eyebrow } from "../../design-system/eyebrow";
import { activityDay, activityTimeOfDay } from "../../format/activitywhen";
import { formatDayMonth, formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { useActivity } from "../../lib/activityread";
import { useRecordZone } from "../../lib/recordzone";
import { navigate } from "../../lib/router";
import {
  briefContextTitle,
  briefSubjectLabel,
  dealFactsText,
  itemTitle,
  phrasedReasons,
  reasonText,
  subjectHref,
  whenText,
} from "../worklist/worklist.copy";
import { eyebrowKeyFor } from "../worklist/worklist.eyebrow";
import type { WorklistItem } from "../worklist/worklist.queries";
import { WorklistRow } from "../worklist/worklist.row";
import { contactHref } from "../worklist/worklist.row.captions";

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
  const task = useActivity(lead.id, lead.source === "task").data;
  const cardVerbs =
    onContext || task ? (
      <>
        {/* Every row opens on what it asks and why it is here. */}
        {onContext && (
          <Button variant="ghost" onClick={() => onContext(lead)}>
            {t("brief.focus.context")}
          </Button>
        )}
        {task && <TaskNextSteps task={task} offerOpen={false} />}
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
    : whenText(lead, t, locale, zone, recordZone, new Date());
  const label = t(eyebrowKeyFor(lead));
  return (
    <div className="brief-triage">
      {/* Keyed by the row, so pressing another one in the list mounts a new
          card and replays its arrival. The press is answered on the card. */}
      <div
        ref={takeFocus}
        tabIndex={-1}
        className="brief-triage-frame"
        key={identity(lead)}
      >
        <Card as="article" className="brief-triage-lead">
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
            // The card around this row says whose it is, why it is here and
            // where it stands in the day. Each is said once, in its own place.
            framed
          />
        </Card>
      </div>
      {rows.length > 1 && (
        <div className="brief-triage-queue">
          <Eyebrow as="h3" className="brief-triage-queue-head">
            {t("brief.first.then")}
          </Eyebrow>
          {/* An ordered list, so the order reaches a screen reader as the
              claim it is; the tile states the rank for everybody else. */}
          <ol className="brief-focus-list">
            {rows.map((item, index) =>
              index === at ? null : (
                <li key={identity(item)}>
                  <QueueEntry
                    item={item}
                    rank={index + 1}
                    onChoose={() => {
                      pressed.current = true;
                      onChoose(item);
                    }}
                  />
                </li>
              ),
            )}
          </ol>
        </div>
      )}
    </div>
  );
}

/** One row still waiting behind the card; a press puts it in hand. */
function QueueEntry({
  item,
  rank,
  onChoose,
}: Readonly<{ item: WorklistItem; rank: number; onChoose: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const line = queueLine(item, t, locale, viewerZone());
  return (
    <button type="button" className="brief-focus-item" onClick={onChoose}>
      <span className="brief-focus-rank t-num">
        {formatNumber(rank, locale)}
      </span>
      <span className="brief-focus-item-title">
        {briefSubjectLabel(item) ?? itemTitle(item, t, locale)}
      </span>
      {line && <span className="t-caption brief-focus-item-line">{line}</span>}
    </button>
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
  const href = contact?.label ? contactHref(contact) : subjectHref(item);
  const inbox = item.email_summary?.display_status;
  return (
    <>
      {label && href && (
        <a className="entity-link" href={href}>
          {label}
        </a>
      )}
      {inbox && <span>{t(`visibility.${inbox}`)}</span>}
    </>
  );
}

/**
 * The line under a queued row's name: who it is with and its first reason,
 * through the row's own helpers. Null when the row has neither.
 */
function queueLine(
  item: WorklistItem,
  t: Translator,
  locale: Locale,
  zone: string,
): string | null {
  // The contact on the row, not the message's counterparty: a task or meeting
  // has no message. On a waiting message the backend puts the sender here.
  const who = item.contact?.label ?? null;
  // The reader's own pin is the queue's reason and not the focus projection's,
  // for the reason the row in hand withholds it (`allowPin`).
  const reason =
    phrasedReasons(item, false)
      .filter((entry) => entry.kind !== "pinned")
      .map((entry) => reasonText(entry, t, locale, zone))
      .find((text) => text !== null) ?? null;
  const subject = briefSubjectLabel(item);
  const action = briefContextTitle(item);
  const facts = subject ? dealFactsText(item, t, locale, zone) : null;
  const parts = [action, who, facts, reason].filter(
    (part): part is string => part != null && part !== "",
  );
  return parts.length > 0 ? parts.join(" · ") : null;
}

/** Whether the panel's foot has anything to say about the rest of the day. */
