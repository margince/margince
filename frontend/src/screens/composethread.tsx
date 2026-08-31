// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The conversation a reply is answering, beside the reply.
 *
 * The composer used to say what it was answering in one sentence and leave the
 * messages themselves on the page behind — which the dialog was covering. A rep
 * checking what was actually promised had to send or discard first. This puts
 * the thread in the drawer, so the reply is written next to the words it
 * answers rather than over them.
 *
 * It renders through the timeline's own row: the conversation IS the record's
 * chronology narrowed to one thread, and a second message row would be a second
 * answer to "how does a message look" — including how a message nobody in this
 * audience may read looks, which the timeline already draws.
 */

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { activityTimeline } from "../design-system/activitytimeline";
import { PendingBody } from "../design-system/atoms";
import { TimelineRow } from "../design-system/composed";
import type { NameOf } from "../design-system/participants";
import { formatDate, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, translatePlural, useLocale, useT } from "../i18n";
import { throwProblem } from "./common";
import type { RelinkKind } from "./compose";
import { groupChronology } from "./timelinegroups";
import "./composethread.css";

type Activity = components["schemas"]["Activity"];

// How much of a long conversation to carry. A reply is written against the
// recent turns; a thread that has run for a year does not need its first
// message in the drawer, and fetching all of it costs the composer's open.
const SHOWN = 12;

// How many conversations to offer as a way in. A reader picking up an account
// is continuing something recent or starting fresh; a list long enough to
// scroll is a list they read instead of writing.
const OFFERED = 5;

// How much of the record's mail to read to find those conversations. Enough
// that five threads are actually five, on an account whose recent traffic is
// one busy exchange.
const SCANNED = 40;

/**
 * The messages of the anchor's thread, newest first.
 *
 * A one-off mail carries no `thread_key` — capture assigns one only where a
 * provider reported a conversation — and that is not an empty thread: it is a
 * conversation of exactly one message, which is the anchor already in hand. So
 * the list read is skipped rather than run with an empty key, which would ask
 * the server for every activity that has no thread.
 */
export function useThreadMessages(anchor: Activity | undefined): {
  messages: Activity[];
  pending: boolean;
} {
  const threadKey = anchor?.thread_key ?? undefined;
  const query = useQuery({
    queryKey: ["compose-thread", threadKey],
    queryFn: async () => {
      const { data, error } = await api.GET("/activities", {
        params: { query: { thread_key: threadKey, limit: SHOWN } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    enabled: Boolean(threadKey),
  });
  if (!anchor) {
    return { messages: [], pending: false };
  }
  if (!threadKey) {
    return { messages: [anchor], pending: false };
  }
  // A read that has not answered yet must not render as a thread of one: the
  // anchor alone looks like a settled answer ("this conversation is a single
  // message") rather than like a list still arriving.
  return {
    messages: query.data?.data ?? [],
    pending: query.isPending,
  };
}

/**
 * The pane. Newest first, matching the chronology it is drawn from — a reply
 * is written against the last thing said, and that belongs at the top where
 * the reader starts.
 */
export function ThreadPane({
  messages,
  pending,
  viewerUserId,
  nameOf,
  named,
  onLeave,
}: Readonly<{
  messages: readonly Activity[];
  pending: boolean;
  viewerUserId?: string;
  nameOf: NameOf;
  /**
   * Whether the reader OPENED this conversation, or the composer found it.
   *
   * A reply names its activity: the reader pressed Reply on a message and the
   * pane is that message's thread. A mail started from the record names none,
   * and the composer anchors it to the account's latest exchange — so the pane
   * is a conversation the reader never asked for, and calling it "this
   * conversation" claimed they had. It is still worth showing, because the
   * send really will continue it; the heading is what has to say so.
   */
  named: boolean;
  /**
   * The way back to the list, when the reader chose this conversation.
   *
   * A choice with no way out is not one: having picked the wrong thread they
   * would otherwise have to close the composer and lose whatever they had
   * typed to reach the other four.
   */
  onLeave?: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  // No `renderActions`: every row's affordance here would be Reply, inside the
  // reply. The rows are the reading, not a place to act from.
  const entries = activityTimeline([...messages], viewerUserId, undefined, {
    nameOf,
    t,
    locale,
  });
  return (
    <section className="compose-thread" aria-labelledby="compose-thread-head">
      <div className="compose-thread-head">
        <h3 id="compose-thread-head" className="t-eyebrow">
          {t(named ? "compose.threadHeading" : "compose.threadContinuing")}
        </h3>
        {onLeave && (
          <button
            type="button"
            className="compose-thread-leave"
            onClick={onLeave}
          >
            {t("compose.threadLeave")}
          </button>
        )}
      </div>
      <div className="compose-thread-scroll">
        {pending ? (
          <PendingBody label={t("compose.threadPending")} lines={3} />
        ) : (
          <ul className="timeline compose-thread-list">
            {entries.map((entry) => (
              <TimelineRow key={entry.id} entry={entry} zone={zone} />
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}

/**
 * The record's recent conversations, newest first, as ways INTO this message.
 *
 * A composer opened from the record used to anchor itself to the account's
 * latest exchange without saying so: the reader pressed the mail button and got
 * a reply to whatever came last, with a To field filled from a thread they had
 * not chosen. The threads are the suggestion now, and picking one is the
 * reader's move.
 *
 * Grouped through the chronology's own grouping rather than by a second pass
 * over `thread_key` here — the record's History draws these same runs, and two
 * answers to "what counts as one conversation" would drift.
 */
export function useRecentConversations(
  entityType: RelinkKind,
  entityId: string,
  enabled: boolean,
  who: Readonly<{
    nameOf: NameOf;
    t: ReturnType<typeof useT>;
    locale: Locale;
  }>,
): { conversations: readonly Conversation[]; pending: boolean } {
  const query = useQuery({
    queryKey: ["compose-recent-threads", entityType, entityId],
    queryFn: async () => {
      const { data, error } = await api.GET("/activities", {
        params: {
          query: {
            entity_type: entityType,
            entity_id: entityId,
            kind: "email",
            limit: SCANNED,
          },
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    enabled,
  });
  // A message this reader is not in the audience for cannot be a way in: its
  // subject and body come back null, and the draft endpoint — which gates on
  // content, not discovery — answers 404 for it. Offered, it would be a blank
  // row that dead-ends the composer.
  const readable = (query.data?.data ?? []).filter(
    (activity) => activity.content_state !== "withheld",
  );
  const entries = activityTimeline(readable, undefined, undefined, who);
  const conversations = groupChronology(entries)
    .slice(0, OFFERED)
    .map((group) => ({
      // The newest member: continuing a conversation means answering its last
      // message, which is the same activity a Reply on the History tab names.
      anchorId: group.id,
      subject: group.entries[0]?.subject ?? group.entries[0]?.title ?? "",
      counterparts: group.entries[0]?.counterparts,
      atIso: group.entries[0]?.atIso ?? "",
      count: group.entries.length,
      partial: group.partial,
    }));
  return { conversations, pending: query.isPending };
}

export type Conversation = {
  anchorId: string;
  subject: string;
  counterparts?: string;
  atIso: string;
  count: number;
  partial: boolean;
};

/**
 * The ways in, offered rather than chosen for the reader.
 *
 * Nothing is preselected: the reader pressed a button that says "write an
 * email", and a thread picked for them is the surprise this replaces.
 */
export function ConversationChoices({
  conversations,
  pending,
  onChoose,
}: Readonly<{
  conversations: readonly Conversation[];
  pending: boolean;
  onChoose: (anchorId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  return (
    <section className="compose-thread" aria-labelledby="compose-choices-head">
      <h3 id="compose-choices-head" className="compose-thread-head t-eyebrow">
        {t("compose.continueHeading")}
      </h3>
      <div className="compose-thread-scroll">
        {pending ? (
          <PendingBody label={t("compose.threadPending")} lines={3} />
        ) : (
          <ul className="compose-choices">
            {conversations.map((conversation) => (
              <li key={conversation.anchorId}>
                <button
                  type="button"
                  className="compose-choice"
                  onClick={() => onChoose(conversation.anchorId)}
                >
                  <span className="compose-choice-subject">
                    {conversation.subject}
                  </span>
                  <span className="compose-choice-meta t-caption">
                    {[
                      conversation.counterparts,
                      formatDate(conversation.atIso, locale, zone),
                      conversation.count > 1
                        ? translatePlural(
                            locale,
                            "compose.messageCount",
                            conversation.count,
                            { count: formatNumber(conversation.count, locale) },
                          )
                        : undefined,
                    ]
                      .filter(Boolean)
                      .join(" · ")}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}
