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
import { GroupedTimelineList } from "../design-system/composed";
import { useT } from "../i18n";

// A conversation is an exchange somebody can answer: mail, or a message on a
// connected transport. A call or a meeting is an event, and a note is the
// rep's own aside — all three stay in the chronicle cuts.
const CONVERSATION_KINDS: ReadonlySet<string> = new Set(["email", "message"]);

export function conversationGroups(
  groups: readonly TimelineGroup[],
): readonly TimelineGroup[] {
  return groups.filter((group) =>
    CONVERSATION_KINDS.has(group.entries[0].kind),
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
