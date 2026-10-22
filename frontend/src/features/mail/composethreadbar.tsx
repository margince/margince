// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Reply, SquarePen } from "lucide-react";
import { useState } from "react";
import type { components } from "../../api/schema";
import { activityTimeline } from "../../design-system/activitytimeline";
import { Button, PendingBody } from "../../design-system/atoms";
import { EmailEntry } from "../../design-system/emailentry";
import { EmailText } from "../../design-system/emailtext";
import { Eyebrow } from "../../design-system/eyebrow";
import type { NameOf } from "../../design-system/participants";
import { Popover } from "../../design-system/popover";
import { SurfaceState } from "../../design-system/surfacestate";
import { TimelineRow } from "../../design-system/timeline";
import { formatDate, formatDateTime, formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import type { ComposeThread } from "./composeanchor";
import type { Conversation, ThreadRead } from "./composethread";
import "./composethread.css";

type Activity = components["schemas"]["Activity"];
type EmailSummary = components["schemas"]["EmailSummary"];

// A time the read did not carry is left out, rather than drawn as an error.
function whenOf(
  iso: string | null | undefined,
  locale: Parameters<typeof formatDateTime>[1],
  zone: string,
): string | undefined {
  return iso ? formatDateTime(iso, locale, zone) : undefined;
}

/** One thread on offer, and whether the draft answers it. */
type Card = Readonly<{
  anchorId: string;
  subject: string;
  counterparts?: string;
  summary?: EmailSummary;
  /** A non-mail anchor has no summary, and is drawn as its timeline row. */
  activity?: Activity;
  atIso: string;
  selected: boolean;
}>;

/**
 * The threads in reading order, newest first, with the one the draft answers
 * marked. A reply to a thread older than the offer still gets its row, at the
 * top. The list shows what the draft answers whatever the offer holds.
 */
export function threadCards(
  conversations: readonly Conversation[],
  answering: string | undefined,
  anchor: Activity | undefined,
): Card[] {
  const cards: Card[] = conversations.map((conversation) => ({
    anchorId: conversation.anchorId,
    subject: conversation.subject,
    counterparts: conversation.counterparts,
    summary: conversation.summary,
    atIso: conversation.atIso,
    selected:
      answering !== undefined && conversation.memberIds.includes(answering),
  }));
  // A note answered by mail is no thread, so the list does not draw it as one.
  const correspondence = anchor?.kind === "email" || anchor?.kind === "message";
  if (
    !answering ||
    cards.some((card) => card.selected) ||
    !anchor?.id ||
    !correspondence
  ) {
    return cards;
  }
  return [
    {
      anchorId: anchor.id,
      subject: anchor.subject ?? "",
      summary: anchor.email_summary ?? undefined,
      activity: anchor,
      atIso: anchor.occurred_at ?? "",
      selected: true,
    },
    ...cards,
  ];
}

/**
 * Which thread the message goes into, as one bar under the window's title.
 * It names the thread being answered, or a new one with others on offer.
 * Pressing it lists them, so the reader moves the draft onto another thread,
 * onto one message of it, or out to a new one.
 */
export function ThreadBar({
  thread,
  answering,
  recordName,
  viewerId,
  nameOf,
  onSelect,
  disabled,
}: Readonly<{
  thread: ComposeThread;
  answering: string | undefined;
  /** Whose threads these are, when the record behind the window says. */
  recordName: string | undefined;
  viewerId: string | undefined;
  nameOf: NameOf;
  onSelect: (id: string | null) => void;
  disabled: boolean;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const [open, setOpen] = useState(false);
  if (!answering && !thread.splitColumns) {
    return null;
  }
  const { recent, anchorRead } = thread;
  const cards = threadCards(
    recent.conversations,
    answering,
    thread.anchorActivity,
  );
  const choose = (id: string | null) => {
    setOpen(false);
    onSelect(id);
  };
  const count = recent.conversations.length;
  return (
    <div className="compose-threadbar-wrap">
      <Popover
        className="compose-threadbar"
        panelClassName="compose-threadmenu"
        dialog
        open={open}
        onOpenChange={setOpen}
        disabled={disabled}
        label={
          <>
            {answering ? (
              <Reply aria-hidden="true" />
            ) : (
              <SquarePen aria-hidden="true" />
            )}
            <span className="compose-threadbar-text">
              {answering ? (
                thread.answeringLine
              ) : (
                <>
                  <strong>{t("compose.newThread")}</strong>
                  {count > 0 &&
                    ` · ${plural("compose.earlierThreads", count, {
                      count: formatNumber(count, locale),
                    })}`}
                </>
              )}
            </span>
            <span className="compose-threadbar-action">
              {t(answering ? "compose.threadChange" : "compose.threadContinue")}
            </span>
          </>
        }
      >
        {answering && (
          <Button variant="ghost" onClick={() => choose(null)}>
            <SquarePen aria-hidden="true" />
            {t("compose.threadLeave")}
          </Button>
        )}
        <Eyebrow as="h2">
          {recordName
            ? t("compose.threadsWith", { name: recordName })
            : t("compose.threads")}
        </Eyebrow>
        {recent.pending ? (
          <PendingBody label={t("compose.threadPending")} lines={3} />
        ) : (
          <SurfaceState
            loadingLabel={t("compose.threadPending")}
            state={recent.failed ? "failed" : "ready"}
            emptyLabel={t("compose.threadPending")}
            detail={{ onRetry: recent.retry }}
          >
            <ul className="compose-threadlist">
              {answering && anchorRead.failed && (
                <li>
                  <SurfaceState
                    loadingLabel={t("compose.threadPending")}
                    state="failed"
                    emptyLabel={t("compose.threadPending")}
                    detail={{ onRetry: anchorRead.retry }}
                  >
                    {null}
                  </SurfaceState>
                </li>
              )}
              {cards.map((card) => (
                <ThreadRow
                  key={card.anchorId}
                  card={card}
                  answering={answering}
                  thread={thread.conversation}
                  viewerUserId={viewerId}
                  nameOf={nameOf}
                  onChoose={choose}
                  disabled={disabled}
                />
              ))}
            </ul>
          </SurfaceState>
        )}
      </Popover>
    </div>
  );
}

// One thread on offer. The one the draft answers lists its other messages
// under it, each a way to move the draft onto that message.
function ThreadRow({
  card,
  answering,
  thread,
  viewerUserId,
  nameOf,
  onChoose,
  disabled,
}: Readonly<{
  card: Card;
  answering: string | undefined;
  thread: ThreadRead;
  viewerUserId: string | undefined;
  nameOf: NameOf;
  onChoose: (id: string) => void;
  disabled: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const others =
    card.selected && !thread.pending && !thread.failed
      ? thread.messages.filter((message) => message.id !== answering)
      : [];
  return (
    <li className="compose-threadrow" data-selected={card.selected}>
      {card.summary ? (
        <EmailEntry
          summary={card.summary}
          timestamp={whenOf(card.atIso, locale, zone)}
          onSelect={() => onChoose(card.anchorId)}
          selected={card.selected}
          disabled={disabled}
        />
      ) : card.activity ? (
        <ul className="timeline compose-thread-list">
          {activityTimeline([card.activity], viewerUserId, undefined, {
            nameOf,
            t,
            locale,
          }).map((entry) => (
            <TimelineRow key={entry.id} entry={entry} zone={zone} />
          ))}
        </ul>
      ) : (
        // A thread read without its summary still names itself: its subject,
        // who it was with and when, as one press target.
        <Button
          className="compose-choice"
          aria-pressed={card.selected}
          onClick={() => onChoose(card.anchorId)}
          disabled={disabled}
        >
          <span className="compose-choice-subject">
            {card.subject || t("email.noSubject")}
          </span>
          <span className="t-caption">
            {[
              card.counterparts,
              card.atIso && formatDate(card.atIso, locale, zone),
            ]
              .filter(Boolean)
              .join(" · ")}
          </span>
        </Button>
      )}
      {others.length > 0 && (
        <ul className="timeline compose-threadlist-others">
          {others.map((message) => (
            <ThreadMessage
              key={message.id}
              message={message}
              viewerUserId={viewerUserId}
              nameOf={nameOf}
              onChoose={onChoose}
              disabled={disabled}
            />
          ))}
        </ul>
      )}
    </li>
  );
}

// One other message of the open thread, as a way to move the draft onto it.
// A message this reader may not read keeps its held place instead.
function ThreadMessage({
  message,
  viewerUserId,
  nameOf,
  onChoose,
  disabled,
}: Readonly<{
  message: Activity;
  viewerUserId: string | undefined;
  nameOf: NameOf;
  onChoose: (id: string) => void;
  disabled: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const summary = message.email_summary;
  if (!summary) {
    return activityTimeline([message], viewerUserId, undefined, {
      nameOf,
      t,
      locale,
    }).map((entry) => <TimelineRow key={entry.id} entry={entry} zone={zone} />);
  }
  const timestamp = whenOf(message.occurred_at, locale, zone) ?? "";
  const held =
    message.content_state === "withheld" ||
    summary.display_status === "withheld";
  return (
    <li
      className="compose-message"
      aria-label={`${message.subject || t("email.noSubject")} · ${timestamp}`}
    >
      {held ? (
        <EmailEntry
          summary={summary}
          timestamp={timestamp}
          whyNotOpenable="withheld"
        />
      ) : (
        <>
          <EmailEntry
            summary={summary}
            timestamp={timestamp}
            onSelect={() => onChoose(message.id)}
            selected={false}
            disabled={disabled}
          />
          {/* Read without moving the draft onto it. */}
          <div className="compose-message-actions">
            <Popover label={t("compose.previewEmail")} onHover>
              <EmailText body={message.body ?? ""} />
            </Popover>
          </div>
        </>
      )}
    </li>
  );
}
