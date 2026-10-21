// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The focus panel's body: the row in hand as a card, and the ranked column
// beside it. The panel's head, states and foot are in brief.feed.tsx.

import { useRef } from "react";
import { Button, Card } from "../../design-system/atoms";
import { Disclosure } from "../../design-system/disclosure";
import { Eyebrow } from "../../design-system/eyebrow";
import { openingCase } from "../../format/collate";
import {
  formatDayMonth,
  formatNumber,
  formatTimeOfDay,
} from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { navigate } from "../../lib/router";
import { consequenceText } from "../worklist/worklist.connection";
import {
  comparisonText,
  itemTitle,
  phrasedReasons,
  reasonText,
  subjectHref,
  whenText,
} from "../worklist/worklist.copy";
import { eyebrowKeyFor } from "../worklist/worklist.eyebrow";
import { hasPane } from "../worklist/worklist.pane";
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
  // A meeting gets the door too. Its subject is the activity, not a contact,
  // so `hasPane` alone would leave it with no way through.
  const context =
    onContext &&
    (hasPane(lead) ||
      lead.source === "task" ||
      lead.source === "meeting_outcome");
  const details = context ? (
    <Button variant="ghost" onClick={() => onContext(lead)}>
      {t("brief.focus.context")}
    </Button>
  ) : undefined;
  // The head's facts come from the helpers the row uses, so the card and the
  // queue agree. A message is dated by its arrival, the rest by its own clock.
  // The reader's own pin is the queue's reason, not the card's.
  const arrived = lead.email_summary?.occurred_at;
  const when = arrived
    ? `${formatDayMonth(arrived, locale, zone)}, ${formatTimeOfDay(arrived, locale, zone)}`
    : whenText(lead, t, locale, zone, recordZone, new Date());
  const weighed = phrasedReasons(lead, when !== null).filter(
    (reason) => reason.kind !== "pinned",
  );
  // The label is the first reason with a figure, else the first weighed:
  // "Waiting 13 days" says more than "they wrote last". The rest stay on the
  // queue's row.
  const strongest = weighed.find((reason) => reason.value) ?? weighed[0];
  const phrased = strongest ? reasonText(strongest, t, locale, zone) : null;
  // The reasons are written as fragments of a line ("waiting 13 days"); as a
  // label one opens a line, and recasing its first letter is the reader's own
  // locale decision, spelled once in `openingCase`.
  const label = phrased ? openingCase(phrased, locale) : t(eyebrowKeyFor(lead));
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
          {/* Why it is here, whose it is, and when: the strongest reason the
              ranking weighed, in the warning ink on a row the day put first,
              then the contact and the moment. */}
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
            context={details}
            // The card around this row says whose it is, why it is here and
            // where it stands in the day. Each is said once, in its own place.
            framed
          />
          <WhyFirst item={lead} />
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
        {itemTitle(item, t, locale)}
      </span>
      {line && <span className="t-caption brief-focus-item-line">{line}</span>}
    </button>
  );
}

/**
 * The whole case for the row in hand, folded. It holds every reason the
 * ranking weighed, why it beat the row under it, and what doing nothing costs.
 *
 * The same sentences the Worklist's rows print, through the same helpers, so
 * the card and the queue cannot argue one row two ways. Nothing to fold, no
 * fold.
 */
function WhyFirst({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const lines = [
    ...phrasedReasons(item, false)
      .filter((reason) => reason.kind !== "pinned")
      .map((reason) => reasonText(reason, t, locale, zone)),
    // A pin is the reader's own order in the queue, not the brief's reason.
    item.above_next?.comparator === "pin"
      ? null
      : comparisonText(item.above_next, t, locale, zone),
    consequenceText(item, t),
  ].filter((line): line is string => line !== null && line !== "");
  if (lines.length === 0) return null;
  return (
    <Disclosure className="brief-why" summary={t("brief.first.why")}>
      <ul className="brief-why-list">
        {lines.map((line) => (
          <li key={line}>{line}</li>
        ))}
      </ul>
    </Disclosure>
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
  const label = contact?.label ?? item.subject?.label;
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
  const parts = [who, reason].filter(
    (part): part is string => part !== null && part !== "",
  );
  return parts.length > 0 ? parts.join(" · ") : null;
}

/** Whether the panel's foot has anything to say about the rest of the day. */
