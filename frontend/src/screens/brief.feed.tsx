// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { ArrowRight } from "lucide-react";
import { useState } from "react";
import { useRecordZone } from "../app/recordzone";
import { navigate } from "../app/router";
import { Avatar, Badge, Button, Card } from "../design-system/atoms";
import { Eyebrow } from "../design-system/eyebrow";
import { OpenEmailDrawer } from "../design-system/openemaildrawer";
import { Panel, PanelBody } from "../design-system/panel";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import {
  type Locale,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../i18n";
import { isBriefUpdate, waitingRows } from "./brief.sentence";
import {
  phrasedReasons,
  reasonText,
  subjectHref,
  whenText,
} from "./worklist.copy";
import { nextUpLine } from "./worklist.emailtitle";
import { eyebrowKeyFor } from "./worklist.eyebrow";
import { worklistLaneHref } from "./worklist.header";
import { hasPane, lastTouch } from "./worklist.pane";
import {
  type Worklist,
  type WorklistItem,
  worklistKey,
} from "./worklist.queries";
import { WorklistRow } from "./worklist.row";
import { contactHref } from "./worklist.row.captions";

import "./worklist.css";
import "./brief.feed.css";

/** One row's identity across renders, the way the queue page spells it. */
function identity(item: WorklistItem): string {
  return `${item.source}-${item.id}`;
}

export function BriefFeed({
  day,
  state,
  changed,
  refreshFailed = false,
  onRetry,
  onContext,
}: Readonly<{
  day: Worklist | undefined;
  state: SectionState;
  changed?: Readonly<{ count: number; href: string }>;
  refreshFailed?: boolean;
  onRetry?: () => void;
  onContext?: (item: WorklistItem) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const [openEmail, setOpenEmail] = useState<string | null>(null);
  // WHICH ROW IS IN HAND, by identity rather than by index: the queue is
  // re-read every minute and a row that was answered leaves it, so an index
  // would silently hand the reader the row that moved up into its place. A
  // chosen row that has gone falls back to the day's own first — the row the
  // page opens on, and the one the sentence over it names.
  const [chosen, setChosen] = useState<string | null>(null);
  const client = useQueryClient();
  const rows = waitingRows(day);
  const at = Math.max(
    0,
    rows.findIndex((item) => identity(item) === chosen),
  );
  const lead = rows[at];
  const partial = Boolean(
    !day?.focus ||
      day?.readings?.more_available ||
      day?.sources_unavailable?.length,
  );
  return (
    <section id="brief-today">
      <Panel
        // THE LEAD PANEL: the one card on the page that asks for a move
        // rather than reporting state, so it wears the accent's band and
        // edge. Nothing else on the Brief may — a second tinted panel is two
        // leads, which is none.
        tone="accent"
        title={t(
          day?.scope === "team" ? "brief.feed.teamTitle" : "brief.feed.title",
        )}
        sub={
          day?.summary
            ? plural("brief.feed.visible", rows.length, {
                count: formatNumber(rows.length, locale),
              })
            : undefined
        }
        titleAction={
          <span className="brief-focus-actions">
            {/* The panel offers the agent's verbs — the reply it drafted, the
                step it prepared — so it wears the badge every panel that
                offers an AI verb wears: what is OFFERED, beside the tone that
                says who wrote the body. */}
            <Badge tone="ai">{t("co.assistant.aiTag")}</Badge>
            {changed && (
              <a className="entity-link" href={changed.href}>
                <Badge>
                  {plural("brief.feed.changedBadge", changed.count, {
                    count: formatNumber(changed.count, locale),
                  })}
                </Badge>
              </a>
            )}
            {/* The way into the whole queue, on the panel's own head: it is
                the panel's one destination, and at the foot of a list of
                verbs it read as a caption. */}
            {day && (
              <a
                className="btn btn-sm brief-focus-open"
                href={worklistLaneHref("all", day.scope)}
              >
                {t("brief.feed.fullWorklist")}
                <ArrowRight size={14} aria-hidden="true" />
              </a>
            )}
          </span>
        }
        footer={day && hasFoot(day) ? <AgendaFoot day={day} /> : undefined}
      >
        {/* Prose standing where the rows will stand pays the pane's inset
            itself: the triage under it carries its own, and a sentence set
            straight into the panel printed against the card's edge. */}
        {refreshFailed && (
          <PanelBody>
            <p role="alert">
              {t("brief.feed.refreshFailed")}{" "}
              <Button variant="ghost" onClick={onRetry}>
                {t("brief.coverage.retry")}
              </Button>
            </p>
          </PanelBody>
        )}
        <SurfaceState
          state={
            state !== "ready"
              ? state
              : rows.length > 0 || partial
                ? "ready"
                : "empty"
          }
          emptyLabel={t("brief.feed.clear")}
          loadingLabel={t("brief.feed.loading")}
        >
          {rows.length === 0 && partial && (
            <PanelBody>
              <p className="t-caption">{t("brief.feed.incomplete")}</p>
            </PanelBody>
          )}
          {lead && (
            <Triage
              rows={rows}
              at={at}
              lead={lead}
              onChoose={(item) => setChosen(identity(item))}
              onOpenEmail={setOpenEmail}
              onContext={onContext}
            />
          )}
        </SurfaceState>
      </Panel>
      <OpenEmailDrawer
        activityId={openEmail}
        zone={viewerZone()}
        onClose={() => setOpenEmail(null)}
        onReplySent={() =>
          void client.invalidateQueries({ queryKey: worklistKey })
        }
      />
    </section>
  );
}

/**
 * The focus, ONE ROW AT A TIME: the row in hand drawn whole, and beside it
 * the ranked queue it was taken from, each row a press that puts it in hand.
 *
 * A reader clears a morning by answering one thing and moving to the next,
 * and the page is shaped like that act: the work on the left at the row's
 * full density — its kind, the message, its reasons, what doing nothing
 * costs, and every verb that answers it — and on the right the order the
 * day put the rest in, so the reader always sees where they are in it. The
 * sentence over the page says "First: X. Then N more", and this is that
 * sentence drawn: X in hand, the N in a column.
 */
function Triage({
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
  const when =
    whenText(lead, t, locale, zone, recordZone, new Date()) ??
    (lead.email_summary
      ? formatDateTime(lead.email_summary.occurred_at, locale, zone)
      : null);
  const weighed = phrasedReasons(lead, when !== null).filter(
    (reason) => reason.kind !== "pinned",
  );
  // The label is the reason that carries a FIGURE where one does — "waiting
  // 13 days" says more on its own than "they wrote last" — else the first.
  const label = weighed.find((reason) => reason.value) ?? weighed[0];
  const reasons = [label, ...weighed.filter((reason) => reason !== label)]
    .filter((reason) => reason !== undefined)
    .map((reason) => reasonText(reason, t, locale, zone))
    .filter((reason): reason is string => reason !== null);
  return (
    <div className="brief-triage">
      {/* THE ROW IN HAND IS A CARD: one thing to answer, drawn whole — its
          state on top, its name, whose it is, the work, the verbs — on its own
          surface beside the column of what comes next. */}
      <Card as="article" className="brief-triage-lead">
        <div className="brief-triage-lead-head">
          {/* Why it is here, as its label: the strongest reason the ranking
              weighed, in the warn tone on a row the day put first; the kind
              of work where the ranking gave none. Then the rest, quieter. */}
          <Badge tone={lead.band === "now" ? "warn" : undefined}>
            {reasons[0] ?? t(eyebrowKeyFor(lead))}
          </Badge>
          {(reasons.length > 1 || when) && (
            <span className="t-caption brief-triage-reasons">
              {[...reasons.slice(1), ...(when ? [when] : [])].join(" · ")}
            </span>
          )}
          <span className="t-caption t-num brief-triage-position">
            {t("brief.focus.position", {
              at: formatNumber(at + 1, locale),
              count: formatNumber(rows.length, locale),
            })}
          </span>
        </div>
        <AboutLine item={lead} />
        <WorklistRow
          // Personal ordering is the queue's, not the focus projection's. No
          // rank either: the head says where the row stands, in words.
          allowPin={false}
          item={lead}
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
  // The person the server put on the row, else the record it is about: the
  // sender of a thread filed under a deal is a person, and the reply goes to
  // them.
  const contact = item.contact;
  const label = contact?.label ?? item.subject?.label;
  const href = contact?.label ? contactHref(contact) : subjectHref(item);
  if (!label || !href) return null;
  const touch = lastTouch(contact?.touch, t, locale, viewerZone());
  return (
    <p className="t-caption brief-triage-about">
      {contact?.label && (
        <Avatar name={contact.label} identity={contact.id} size="xs" />
      )}
      <a className="entity-link" href={href}>
        {label}
      </a>
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
function hasFoot(day: Worklist): boolean {
  return (
    (day.focus?.urgent_remaining ?? 0) > 0 ||
    (day.focus !== undefined && day.focus.total > day.focus.items.length)
  );
}

function AgendaFoot({ day }: Readonly<{ day: Worklist }>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const hidden = day.focus?.urgent_remaining ?? 0;
  return (
    <>
      {hidden > 0 && (
        <a className="entity-link" href={worklistLaneHref("urgent", day.scope)}>
          {plural("brief.focus.urgentRemaining", hidden, {
            count: formatNumber(hidden, locale),
          })}
        </a>
      )}
      {day.focus && day.focus.total > day.focus.items.length && (
        <span className="t-caption">
          {t("brief.focus.remaining", {
            count: formatNumber(
              day.focus.total - day.focus.items.length,
              locale,
            ),
          })}
        </span>
      )}
    </>
  );
}

/** The plain feed: every row at the queue's own density, in the day's order. */
function AgendaRows({
  rows,
  onOpenEmail,
}: Readonly<{
  rows: readonly WorklistItem[];
  onOpenEmail: (id: string) => void;
}>) {
  // A list of nothing is only its own padding: under the sentence saying the
  // read was incomplete it stood as a blank band the height of two gutters.
  if (rows.length === 0) return null;
  return (
    <ol className="brief-feed-list">
      {rows.map((item) => (
        <li key={identity(item)}>
          <WorklistRow
            item={item}
            density="compact"
            owner=""
            onOpenEmail={onOpenEmail}
            onReview={() =>
              navigate(
                { screen: "worklist" },
                new Map([["filter", item.category]]),
              )
            }
          />
        </li>
      ))}
    </ol>
  );
}

export function BriefUpdates({ day }: Readonly<{ day: Worklist | undefined }>) {
  const t = useT();
  const rows = (day?.queue ?? []).filter(isBriefUpdate);
  const [openEmail, setOpenEmail] = useState<string | null>(null);
  const client = useQueryClient();
  if (rows.length === 0) return null;
  return (
    <Panel title={t("brief.updates.title")}>
      <AgendaRows rows={rows} onOpenEmail={setOpenEmail} />
      <OpenEmailDrawer
        activityId={openEmail}
        zone={viewerZone()}
        onClose={() => setOpenEmail(null)}
        onReplySent={() =>
          void client.invalidateQueries({ queryKey: worklistKey })
        }
      />
    </Panel>
  );
}
