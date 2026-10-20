// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The focus panel's inside: the row in hand as a card on a stack, and the
// ranked column beside it. The panel's chrome (head, states, foot) is
// brief.feed.tsx.

import { Avatar, Badge, Button, Card } from "../../design-system/atoms";
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
import {
  itemTitle,
  phrasedReasons,
  reasonText,
  subjectHref,
  whenText,
} from "../worklist/worklist.copy";
import { eyebrowKeyFor } from "../worklist/worklist.eyebrow";
import { hasPane, lastTouch } from "../worklist/worklist.pane";
import type { WorklistItem } from "../worklist/worklist.queries";
import { WorklistRow } from "../worklist/worklist.row";
import { contactHref, rowTouch } from "../worklist/worklist.row.captions";

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
  // A meeting earns the door too. Its subject is the activity, not a contact,
  // so `hasPane` answers no for it.
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
  // The head's facts, off the same helpers the row prints them with, so the
  // card and the queue cannot describe one piece of work two ways. A message
  // is dated by its arrival, day and hour; everything else by its own clock.
  // The reader's own pin is the queue's reason, not the focus projection's
  // (`allowPin`).
  const arrived = lead.email_summary?.occurred_at;
  const when = arrived
    ? `${formatDayMonth(arrived, locale, zone)}, ${formatTimeOfDay(arrived, locale, zone)}`
    : whenText(lead, t, locale, zone, recordZone, new Date());
  const weighed = phrasedReasons(lead, when !== null).filter(
    (reason) => reason.kind !== "pinned",
  );
  // The label is the reason that carries a figure ("Waiting 13 days"), else
  // the first weighed. The rest stay on the queue's row, behind the fold.
  const strongest = weighed.find((reason) => reason.value) ?? weighed[0];
  const phrased = strongest ? reasonText(strongest, t, locale, zone) : null;
  // The reasons are written as fragments of a line ("waiting 13 days"); as a
  // label one opens a line, and recasing its first letter is the reader's own
  // locale decision, spelled once in `openingCase`.
  const label = phrased ? openingCase(phrased, locale) : t(eyebrowKeyFor(lead));
  return (
    <div className="brief-triage">
      {/* The row in hand is a card on a stack. The edges of the work behind
          it show, so a reader sees the day has more. Depth is what remains, at
          most two edges: a third reads as texture, not a count.

          Keyed by the row, so pressing another mounts a new card and replays
          its arrival, an answer on the card as well as in the column. */}
      <div
        className="brief-triage-stack"
        data-behind={Math.min(rows.length - 1, 2)}
      >
        <Card as="article" className="brief-triage-lead" key={identity(lead)}>
          <div className="brief-triage-lead-head">
            {/* Why it is here: the strongest reason the ranking weighed, in
              the warning tone on a row the day put first; else the kind of work. */}
            <Badge tone={lead.band === "now" ? "warning" : undefined}>
              {label}
            </Badge>
            {when && <span className="t-caption">{when}</span>}
            <Eyebrow className="t-num brief-triage-position">
              {t("brief.focus.position", {
                at: formatNumber(at + 1, locale),
                count: formatNumber(rows.length, locale),
              })}
            </Eyebrow>
          </div>
          <AboutLine item={lead} />
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
            // The card around this row says whose it is and why it is here.
            // It also gives its place in the day and links the record.
            framed
          />
        </Card>
      </div>
      <div className="brief-triage-queue">
        <Eyebrow as="h3" className="brief-triage-queue-head">
          {t("brief.focus.inQueue")}
        </Eyebrow>
        {/* An ordered list, so the order reaches a screen reader as the
            claim it is; the tile states the rank for everybody else. */}
        <ol className="brief-focus-list">
          {rows.map((item, index) => {
            const line = queueLine(item, t, locale, zone);
            return (
              <li key={identity(item)}>
                <button
                  type="button"
                  className={
                    index === at
                      ? "brief-focus-item brief-focus-item-inhand"
                      : "brief-focus-item"
                  }
                  aria-pressed={index === at}
                  onClick={() => onChoose(item)}
                >
                  <span className="brief-focus-rank t-label t-num">
                    {formatNumber(index + 1, locale)}
                  </span>
                  <span className="brief-focus-item-text">
                    <span className="t-body brief-focus-item-title">
                      {itemTitle(item, t, locale)}
                    </span>
                    {line && (
                      <span className="t-caption brief-focus-item-line">
                        {line}
                      </span>
                    )}
                  </span>
                </button>
              </li>
            );
          })}
        </ol>
      </div>
    </div>
  );
}

/**
 * Whose row this is, and how the silence runs in both directions, over the
 * row, where a reader meets the relationship before the work.
 *
 * The record the row is about, linked. It carries the two moments a rep would
 * otherwise open a second page for: when they last wrote, and when we did.
 * Which direction went last is the whole question. The moments come off the row
 * itself (`rowTouch`), so the row and the pane cannot describe one
 * relationship two ways. A contact's row and a company's carry them; any other
 * record is linked with no moments.
 */
function AboutLine({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const { locale } = useLocale();
  // The contact the server put on the row, else the record it is about: the
  // sender of a thread filed under a deal is a contact, and the reply goes
  // to them.
  const contact = item.contact;
  const label = contact?.label ?? item.subject?.label;
  const href = contact?.label ? contactHref(contact) : subjectHref(item);
  if (!label || !href) return null;
  const touch = lastTouch(rowTouch(item), t, locale, viewerZone());
  // Where the message sits (team inbox or the reader's own), in the access
  // badge's words.
  const inbox = item.email_summary?.display_status;
  return (
    <p className="t-caption brief-triage-about">
      {contact?.label && <Avatar name={contact.label} identity={contact.id} />}
      <a className="entity-link" href={href}>
        {label}
      </a>
      {inbox && (
        <span className="brief-triage-inbox">{t(`visibility.${inbox}`)}</span>
      )}
      {touch.map((fact) => (
        <span key={fact.term} className="brief-triage-about-fact">
          <span>{fact.term}</span> <span className="t-num">{fact.value}</span>
        </span>
      ))}
    </p>
  );
}

/**
 * The one line under a queued row's name: who it is with, and the first of
 * the reasons it is here. It uses the row's own helpers, so the column and the
 * row in hand cannot describe one piece of work two ways. Null where the row
 * has neither, so no empty line is drawn.
 */
function queueLine(
  item: WorklistItem,
  t: Translator,
  locale: Locale,
  zone: string,
): string | null {
  // The contact the server put on the row, not the message's counterparty: a
  // task or a meeting carries a contact and no message. On a waiting message
  // the two are the same, so this one field serves every row, as in the row's
  // own captions (`aboutRecord`).
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
