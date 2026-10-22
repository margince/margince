// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { api, FIRST_PAGE } from "../../api/client";
import type { components } from "../../api/schema";
import { activityTimeline } from "../../design-system/activitytimeline";
import type { NameOf } from "../../design-system/participants";
import { groupChronology } from "../../design-system/timelinegroups";
import type { Locale, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import type { RelinkKind } from "./compose";

type Activity = components["schemas"]["Activity"];
type EmailSummary = components["schemas"]["EmailSummary"];

// How much of a long conversation to carry. A reply is written against the
// recent turns; a thread that has run for a year does not need its first
// message in the drawer, and fetching all of it costs the composer's open.
const SHOWN = 12;

// How much of the record's mail one page reads to find conversations. A record
// with a hundred threads is paged and searched rather than read whole.
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
export function useThreadMessages(anchor: Activity | undefined): ThreadRead {
  const threadKey = anchor?.thread_key ?? undefined;
  const query = useQuery({
    queryKey: ["compose-thread", threadKey],
    queryFn: async () => {
      return unwrap(
        await api.GET("/activities", {
          params: { query: { thread_key: threadKey, limit: SHOWN } },
        }),
      );
    },
    enabled: Boolean(threadKey),
  });
  const retry = () => {
    void query.refetch();
  };
  if (!anchor?.id) {
    return { messages: [], pending: false, failed: false, retry };
  }
  if (!threadKey) {
    return { messages: [anchor], pending: false, failed: false, retry };
  }
  // A read that has not answered yet must not render as a thread of one: the
  // anchor alone looks like a settled answer ("this conversation is a single
  // message") rather than like a list still arriving. Nor may one that FAILED
  // render as a thread of none: an empty list is a settled answer too, and the
  // pane would then claim the conversation is a single message it cannot show.
  return {
    // An older selected anchor may sit outside the newest-message page.
    messages: query.data?.data
      ? query.data.data.some((message) => message.id === anchor.id)
        ? query.data.data
        : [...query.data.data, anchor]
      : [],
    pending: query.isPending,
    failed: query.isError,
    retry,
  };
}

/**
 * What a thread read knows: its rows, and whether the read is still out or
 * came back refused. `failed` and `pending` are never both true, and a pane
 * that draws neither draws the rows.
 */
export type ThreadRead = {
  messages: Activity[];
  pending: boolean;
  failed: boolean;
  retry: () => void;
};

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
  /** Words the threads must hold; empty lists them all, newest first. */
  search = "",
): {
  conversations: readonly Conversation[];
  pending: boolean;
  failed: boolean;
  retry: () => void;
  /** Older mail is still unread; `loadMore` reads its next page. */
  more: boolean;
  loadMore: () => void;
  loadingMore: boolean;
  moreFailed: boolean;
} {
  const term = search.trim();
  const query = useInfiniteQuery({
    queryKey: ["compose-recent-threads", entityType, entityId, term],
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api.GET("/activities", {
          params: {
            query: {
              entity_type: entityType,
              entity_id: entityId,
              kind: "email",
              limit: SCANNED,
              ...(pageParam ? { cursor: pageParam } : {}),
              ...(term ? { q: term } : {}),
            },
          },
        }),
      ),
    initialPageParam: FIRST_PAGE,
    // A response without its page block is read as the last page.
    getNextPageParam: (last) =>
      last.page?.has_more && last.page.next_cursor
        ? last.page.next_cursor
        : undefined,
    enabled,
  });
  // A message this reader is not in the audience for cannot be a way in: its
  // subject and body come back null, and the draft endpoint — which gates on
  // content, not discovery — answers 404 for it. Offered, it would be a blank
  // row that dead-ends the composer.
  const loaded = (query.data?.pages ?? []).flatMap((page) => page.data ?? []);
  const readable = loaded.filter(
    (activity) => activity.content_state !== "withheld",
  );
  const entries = activityTimeline(readable, undefined, undefined, who);
  // A bulk send is not a conversation. It is one message the sender addressed
  // to several contacts and has no thread of its own, so "continuing" it would
  // anchor a reply to a mailing nobody wrote back to. Threads and single
  // messages are the ways in; the bulk groups are left where the History tab
  // draws them.
  const conversations = groupChronology(entries)
    .filter((group) => group.kind !== "bulk")
    .map((group) => ({
      // The newest member: continuing a conversation means answering its last
      // message, which is the same activity a Reply on the History tab names.
      anchorId: group.id,
      subject: group.entries[0]?.subject ?? group.entries[0]?.title ?? "",
      counterparts: group.entries[0]?.counterparts,
      atIso: group.entries[0]?.atIso ?? "",
      count: group.entries.length,
      partial: group.partial,
      summary: group.entries[0]?.emailSummary,
      // Every message the run holds, so a reply to an older one still finds
      // the conversation it belongs to.
      memberIds: group.entries.map((entry) => entry.id),
    }));
  // A failed read is reported as one rather than as a record with no history:
  // folded into an empty list it would send the reader to a fresh mail with
  // nothing said, which is the surprise the offer exists to prevent.
  return {
    conversations,
    pending: query.isPending,
    // A failed older page keeps the threads already read on screen; only a
    // failed first read is a failed list.
    failed: query.isLoadingError,
    retry: () => {
      void query.refetch();
    },
    more: query.hasNextPage,
    loadMore: () => {
      void query.fetchNextPage();
    },
    loadingMore: query.isFetchingNextPage,
    moreFailed: query.isFetchNextPageError,
  };
}

export type Conversation = {
  anchorId: string;
  subject: string;
  counterparts?: string;
  atIso: string;
  count: number;
  partial: boolean;
  summary?: EmailSummary;
  memberIds: readonly string[];
};
