// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { SquarePen } from "lucide-react";
import { useState } from "react";
import type { components } from "../../api/schema";
import { activityTimeline } from "../../design-system/activitytimeline";
import { Button, PendingBody } from "../../design-system/atoms";
import { EmailEntry } from "../../design-system/emailentry";
import { EmailText } from "../../design-system/emailtext";
import { Heading } from "../../design-system/heading";
import type { NameOf } from "../../design-system/participants";
import { Popover } from "../../design-system/popover";
import { SurfaceState } from "../../design-system/surfacestate";
import { TimelineRow } from "../../design-system/timeline";
import { formatDate, formatDateTime, formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
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

/** One card on the rail: a conversation, and whether the draft answers it. */
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
 * The cards in reading order: the offered conversations, newest first, with
 * the one the draft answers marked. A reply to a conversation older than the
 * offer still gets its card, at the top. The rail shows what the draft answers
 * whatever the offer holds.
 */
export function railCards(
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
  // A note answered by mail is no thread, so the rail does not draw it as one.
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
 * The record's conversations beside the draft, as ways into it.
 *
 * Always drawn while there is history, on a reply as on a new message. A
 * reader answering one thread can see the others and move the draft to one. A
 * reader starting fresh can continue one instead. The card the draft answers
 * is marked, and opens to the words it answers.
 */
export function ConversationRail({
  recordName,
  conversations,
  pending,
  failed,
  onRetry,
  answering,
  anchor,
  anchorPending,
  anchorFailed,
  onRetryAnchor,
  thread,
  viewerUserId,
  nameOf,
  onChoose,
  onLeave,
  disabled,
}: Readonly<{
  /** Whose conversations these are, when the record behind the drawer says. */
  recordName: string | undefined;
  conversations: readonly Conversation[];
  pending: boolean;
  failed: boolean;
  onRetry: () => void;
  answering: string | undefined;
  /** The message the draft answers, once read. */
  anchor: Activity | undefined;
  anchorPending: boolean;
  anchorFailed: boolean;
  onRetryAnchor: () => void;
  /** The answered conversation's messages, for the open card. */
  thread: ThreadRead;
  viewerUserId: string | undefined;
  nameOf: NameOf;
  onChoose: (id: string) => void;
  onLeave: () => void;
  disabled: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const cards = railCards(conversations, answering, anchor);
  return (
    <section className="compose-thread" aria-labelledby="compose-rail-head">
      <div className="compose-thread-head">
        <Heading size="xsmall" id="compose-rail-head">
          {recordName
            ? t("compose.railHeadingNamed", { name: recordName })
            : t("compose.railHeading")}
        </Heading>
        {!pending && !failed && (
          <span className="t-caption compose-rail-count">
            {formatNumber(cards.length, locale)}
          </span>
        )}
      </div>
      <div className="compose-thread-scroll">
        {pending ? (
          <PendingBody label={t("compose.threadPending")} lines={3} />
        ) : (
          <SurfaceState
            loadingLabel={t("compose.threadPending")}
            state={failed ? "failed" : "ready"}
            emptyLabel={t("compose.threadPending")}
            detail={{ onRetry }}
          >
            <ul className="compose-rail">
              {answering &&
                (anchorPending || anchorFailed) &&
                !cards.some((card) => card.selected) && (
                  <li className="compose-rail-card" data-selected="true">
                    {anchorFailed ? (
                      <SurfaceState
                        loadingLabel={t("compose.threadPending")}
                        state="failed"
                        emptyLabel={t("compose.threadPending")}
                        detail={{ onRetry: onRetryAnchor }}
                      >
                        {null}
                      </SurfaceState>
                    ) : (
                      anchorPending && (
                        <PendingBody
                          label={t("compose.threadPending")}
                          lines={2}
                        />
                      )
                    )}
                  </li>
                )}
              {cards.map((card) => (
                <RailCard
                  key={card.anchorId}
                  card={card}
                  answering={answering}
                  thread={thread}
                  anchorFailed={anchorFailed}
                  onRetryAnchor={onRetryAnchor}
                  viewerUserId={viewerUserId}
                  nameOf={nameOf}
                  onChoose={onChoose}
                  disabled={disabled}
                />
              ))}
            </ul>
          </SurfaceState>
        )}
        {answering && (
          <Button
            variant="ghost"
            className="compose-rail-leave"
            onClick={onLeave}
            disabled={disabled}
          >
            <SquarePen aria-hidden="true" />
            {t("compose.threadLeave")}
          </Button>
        )}
      </div>
    </section>
  );
}

function RailCard({
  card,
  answering,
  thread,
  anchorFailed,
  onRetryAnchor,
  viewerUserId,
  nameOf,
  onChoose,
  disabled,
}: Readonly<{
  card: Card;
  answering: string | undefined;
  thread: ThreadRead;
  anchorFailed: boolean;
  onRetryAnchor: () => void;
  viewerUserId: string | undefined;
  nameOf: NameOf;
  onChoose: (id: string) => void;
  disabled: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  // Whether the marked card shows the words it answers, or only its summary.
  const [reading, setReading] = useState(false);
  const withheld = card.summary?.display_status === "withheld";
  return (
    <li className="compose-rail-card" data-selected={card.selected}>
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
        // A conversation read without its summary still names itself: its
        // subject, who it was with and when, as one press target.
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
      {card.selected && anchorFailed && (
        <SurfaceState
          loadingLabel={t("compose.threadPending")}
          state="failed"
          emptyLabel={t("compose.threadPending")}
          detail={{ onRetry: onRetryAnchor }}
        >
          {null}
        </SurfaceState>
      )}
      {card.selected && !anchorFailed && (
        <ThreadReading
          reading={reading}
          answering={answering}
          thread={thread}
          viewerUserId={viewerUserId}
          nameOf={nameOf}
          onChoose={onChoose}
          disabled={disabled}
        />
      )}
      {!withheld && (
        <div className="compose-rail-actions">
          {card.selected ? (
            <Button
              variant="link"
              aria-expanded={reading}
              onClick={() => setReading((was) => !was)}
            >
              {t(reading ? "compose.threadSummary" : "compose.readEmail")}
            </Button>
          ) : (
            <Button
              variant="link"
              onClick={() => onChoose(card.anchorId)}
              disabled={disabled}
            >
              {t("compose.replyInThread")}
            </Button>
          )}
        </div>
      )}
    </li>
  );
}

/**
 * The marked card's body. The words the draft answers come first, when the
 * reader opens them. The thread's other messages follow, each a way in.
 */
function ThreadReading({
  reading,
  answering,
  thread,
  viewerUserId,
  nameOf,
  onChoose,
  disabled,
}: Readonly<{
  reading: boolean;
  answering: string | undefined;
  thread: ThreadRead;
  viewerUserId: string | undefined;
  nameOf: NameOf;
  onChoose: (id: string) => void;
  disabled: boolean;
}>) {
  const t = useT();
  if (thread.pending) {
    return <PendingBody label={t("compose.threadPending")} lines={3} />;
  }
  if (thread.failed) {
    return (
      <SurfaceState
        loadingLabel={t("compose.threadPending")}
        state="failed"
        emptyLabel={t("compose.threadPending")}
        detail={{ onRetry: thread.retry }}
      >
        {null}
      </SurfaceState>
    );
  }
  const answered = thread.messages.find((message) => message.id === answering);
  // A message this reader may not read keeps its place, so the thread does not
  // look continuous where it is not.
  const others = thread.messages.filter((message) => message.id !== answering);
  return (
    <div className="compose-rail-reading">
      {reading && answered && answered.content_state !== "withheld" && (
        <EmailText body={answered.body ?? ""} />
      )}
      {others.length > 0 && (
        <ul className="timeline compose-rail-others">
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
    </div>
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
