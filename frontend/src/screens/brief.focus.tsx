// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// THE FOCUS PANEL'S INSIDE: the row in hand as a card, on the stack of what
// is still behind it, and the ranked column beside it. The panel's own chrome
// — its head, its states, its foot — is brief.feed.tsx; this is what stands
// in its body.

import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { Avatar, Badge, Button, Card } from "../design-system/atoms";
import { Eyebrow } from "../design-system/eyebrow";
import { openingCase } from "../format/collate";
import {
  formatDayMonth,
  formatNumber,
  formatTimeOfDay,
} from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../i18n";
import {
  phrasedReasons,
  reasonText,
  subjectHref,
  whenText,
} from "./worklist.copy";
import { nextUpLine } from "./worklist.emailtitle";
import { eyebrowKeyFor } from "./worklist.eyebrow";
import { hasPane, lastTouch } from "./worklist.pane";
import type { WorklistItem } from "./worklist.queries";
import { WorklistRow } from "./worklist.row";
import { contactHref } from "./worklist.row.captions";

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
  // A meeting earns the door too. Its subject is the ACTIVITY rather than a
  // contact, so `hasPane` answers no for it — and the row would stand with
  // three verbs and no way through to the meeting they are about.
  const context =
    onContext &&
    (hasPane(lead) ||
      lead.source === "task" ||
      lead.source === "meeting_outcome");
  const details = context ? (
    <Button small variant="ghost" onClick={() => onContext(lead)}>
      {t("brief.focus.context")}
    </Button>
  ) : undefined;
  // The head's facts, off the same helpers the row prints them with, so the
  // card and the queue cannot describe one piece of work two ways. The
  // reader's own pin is the queue's reason and not the focus projection's
  // (`allowPin`).
  // The head's facts, off the same helpers the row prints them with, so the
  // card and the queue cannot describe one piece of work two ways. A message
  // is dated by when it arrived, day and hour; everything else by its own
  // clock. The reader's own pin is the queue's reason and not the focus
  // projection's (`allowPin`).
  const arrived = lead.email_summary?.occurred_at;
  const when = arrived
    ? `${formatDayMonth(arrived, locale, zone)}, ${formatTimeOfDay(arrived, locale, zone)}`
    : whenText(lead, t, locale, zone, recordZone, new Date());
  const weighed = phrasedReasons(lead, when !== null).filter(
    (reason) => reason.kind !== "pinned",
  );
  // The label is the ONE reason that carries a figure where one does —
  // "Waiting 13 days" says more on its own than "they wrote last" — else the
  // first weighed; the rest stay on the queue's row, where a reader who wants
  // the whole case opens the fold. Sentence case, because it is a label and
  // not a fragment of a line.
  const strongest = weighed.find((reason) => reason.value) ?? weighed[0];
  const phrased = strongest ? reasonText(strongest, t, locale, zone) : null;
  // The reasons are written as fragments of a line ("waiting 13 days"); as a
  // label one opens a line, and recasing its first letter is the reader's own
  // locale decision, spelled once in `openingCase`.
  const label = phrased ? openingCase(phrased, locale) : t(eyebrowKeyFor(lead));
  return (
    <div className="brief-triage">
      {/* THE ROW IN HAND IS A CARD on a STACK: one thing to answer, drawn
          whole — its state on top, its name, whose it is, the work, the verbs
          — over the edges of the work still behind it, so a reader sees the
          day has more without reading the column beside it. The stack's
          depth is what remains, at most two edges: a third reads as texture
          rather than as a count, and the column names them all anyway.

          KEYED BY THE ROW, so pressing another one in the queue mounts a new
          card and replays its arrival — the press has an answer on the card
          itself, not only in the column that was pressed. */}
      <div
        className="brief-triage-stack"
        data-behind={Math.min(rows.length - 1, 2)}
      >
        <Card as="article" className="brief-triage-lead" key={identity(lead)}>
          <div className="brief-triage-lead-head">
            {/* Why it is here, as its label: the strongest reason the ranking
              weighed, in the warn tone on a row the day put first; the kind
              of work where the ranking gave none. Then the rest, quieter. */}
            <Badge tone={lead.band === "now" ? "warn" : undefined}>
              {label}
            </Badge>
            {when && (
              <span className="t-caption brief-triage-when">{when}</span>
            )}
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
            acts="triage"
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
                      {nextUpLine(item, t, locale)}
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
 * WHOSE row this is, and how the silence runs in both directions — over the
 * row, where a reader meets the relationship before the work.
 *
 * The record the row is about, linked, with the two moments a rep answering
 * it would otherwise open a second page for: when they last wrote, and when
 * we did. Which direction went last is the whole question — a customer we
 * mailed yesterday is answered differently from one nobody has written to
 * since March. The moments come off the contact's own 360 read, the SAME
 * read the pane beside the queue makes and the same words it prints, so the
 * row in hand and the pane cannot describe one relationship two ways; and
 * because it is the same key, opening the pane afterwards costs no request.
 * A record that is not a contact is linked and no moments are claimed.
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
  const touch = lastTouch(contact?.touch, t, locale, viewerZone());
  // Where the message sits — the team's inbox, the reader's own — in the
  // access badge's own words, as one more fact about whose row it is.
  const inbox = item.email_summary?.display_status;
  return (
    <p className="t-caption brief-triage-about">
      {contact?.label && (
        <Avatar name={contact.label} identity={contact.id} size="xs" />
      )}
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
 * the reasons it is here — the same words the row itself prints, through the
 * same helpers, so the column and the row in hand cannot describe one piece
 * of work two ways. Null where the row has neither, and then no line is
 * drawn rather than an empty one.
 */
function queueLine(
  item: WorklistItem,
  t: Translator,
  locale: Locale,
  zone: string,
): string | null {
  const who = item.email_summary?.counterparty ?? null;
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
