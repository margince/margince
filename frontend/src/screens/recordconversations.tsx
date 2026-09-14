// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The record's exchanges as CONVERSATIONS: the same thread and message rows
// the full chronology draws, narrowed to the exchanges somebody can answer.
//
// A cut of the chronicle, not a second rendering of it. The rows come from
// the same groups and the same GroupedTimelineList as the All view, so a
// conversation reads IDENTICALLY on both cuts — same whose-move flag, same
// preview, same expansion — and the only thing this cut does is leave out
// what is not a conversation: calls, meetings, notes, tasks and field
// changes.

import { EmptyState } from "../design-system/atoms";
import type { TimelineGroup } from "../design-system/composed";
import {
  GroupedTimelineList,
  isConversationKind,
} from "../design-system/composed";
import {
  ACTIVITY_KINDS,
  type ActivityKind,
  type TimelineFilters,
  useTimelineFilters,
} from "../design-system/recordtimeline";
import { useT } from "../i18n";
import { type TimelineFilter, useChronologyFilter } from "./recordchronology";

/**
 * The kinds this cut can draw, for the dial that narrows it. DERIVED from the
 * two lists that already decide the answer — every kind the activity list
 * accepts, asked the timeline's own word for a conversation — rather than
 * spelled a third time here: a transport added to either reaches this dial
 * without anybody remembering to add it.
 */
export const CONVERSATION_FILTER_KINDS: readonly ActivityKind[] =
  ACTIVITY_KINDS.filter((kind) => isConversationKind(kind));

/**
 * conversationFilters drops a narrowing this cut cannot honour.
 *
 * The kind dial is a SERVER parameter and the cut is a client-side narrowing
 * of what comes back, so the two can contradict each other: a reader who
 * picked Meetings on the whole chronology and then opened Conversations had
 * the server answering about meetings and this cut throwing every row away —
 * an empty list on a record whose mail stands under the next pill.
 */
export function conversationFilters(filters: TimelineFilters): TimelineFilters {
  if (!filters.kind || isConversationKind(filters.kind)) {
    return filters;
  }
  return { ...filters, kind: undefined };
}

/**
 * useChronologyCut holds how a record's chronology is being read: which cut,
 * and the dials narrowing it.
 *
 * The two are ONE state because this cut ties them together — it draws mail
 * and messages, so it cannot honour every kind the dial can send — and a page
 * wiring them separately has to remember the tie. Both pages that offer the cut
 * had the same nine lines, which is nine lines each to get wrong.
 */
export function useChronologyCut(recordId: string): Readonly<{
  filter: TimelineFilter;
  openCut: (next: TimelineFilter) => void;
  filters: TimelineFilters;
  setFilters: (next: TimelineFilters) => void;
  /** The kinds THIS cut can draw, or every kind. */
  kinds: readonly ActivityKind[] | undefined;
}> {
  const [filter, setFilter] = useChronologyFilter(recordId);
  const [filters, setFilters] = useTimelineFilters(recordId);
  return {
    filter,
    filters,
    setFilters,
    // Opening a cut narrows what the reader asked for rather than
    // contradicting it: a kind this cut cannot draw stands down as it opens.
    openCut: (next) => {
      setFilter(next);
      if (next === "conversations") {
        setFilters(conversationFilters(filters));
      }
    },
    kinds: filter === "conversations" ? CONVERSATION_FILTER_KINDS : undefined,
  };
}

// What counts as a conversation is the timeline's own vocabulary
// (isConversationKind), asked of the group's newest entry — optional-chained
// because the type admits an empty group even though groupChronology never
// builds one, and a crash over an impossible shape would take the cut down
// with it.
export function conversationGroups(
  groups: readonly TimelineGroup[],
): readonly TimelineGroup[] {
  return groups.filter((group) =>
    isConversationKind(group.entries[0]?.kind ?? ""),
  );
}

/**
 * ConversationList draws the conversation rows, or says honestly that this
 * record holds none. It is handed EVERY group and cuts to the conversations
 * itself, so the caller cannot hand it a cut that disagrees with
 * `conversationGroups`.
 */
export function ConversationList({
  groups,
  zone,
}: Readonly<{ groups: readonly TimelineGroup[]; zone: string }>) {
  const t = useT();
  const conversations = conversationGroups(groups);
  if (conversations.length === 0) {
    return <EmptyState>{t("chronology.conversationsEmpty")}</EmptyState>;
  }
  return (
    <div className="timeline-card">
      <GroupedTimelineList groups={conversations} zone={zone} />
    </div>
  );
}
