// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { type Dispatch, type SetStateAction, useEffect, useRef } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { formatDateTime } from "../format/format";
import { replySubject } from "../format/replysubject";
import { viewerZone } from "../format/timezone";
import { type Locale, useLocale, useT } from "../i18n";
import { unwrap, useViewerId } from "./common";
import { recordNamesIn, useCompany360 } from "./company360";
import { keptTag } from "./composedraftcall";
import type { RelinkKind } from "./composerelink";
import { useRecentConversations, useThreadMessages } from "./composethread";
import { useRoster } from "./entityref";
import { stripEveryKeyTag, withSubjectTag } from "./projectrecord";

type Activity = components["schemas"]["Activity"];
type ThreadProject = ReturnType<typeof useThreadProject>;

/**
 * The anchor activity and the project link its thread carries, if any.
 *
 * A filed sibling message is the settled answer, and it outranks what the
 * deal says. It is the first rung of the ladder the capture side walks.
 */
export function useThreadProject(
  activityId?: string,
  enabled = true,
): {
  activity?: Activity;
  projectId?: string;
  settled: boolean;
  failed: boolean;
  retry: () => void;
} {
  const query = useQuery({
    queryKey: ["activity", activityId, "filing"],
    queryFn: async () => {
      return unwrap(
        await api.GET("/activities/{id}", {
          params: { path: { id: activityId ?? "" } },
        }),
      );
    },
    enabled: enabled && Boolean(activityId),
  });
  return {
    // The anchor itself too: the pane needs the `thread_key` this read carries.
    activity: query.data,
    retry: () => {
      void query.refetch();
    },
    // Apart from `settled`: the filing says nothing on a failure, while the
    // pane stops holding its place.
    failed: query.isError,
    projectId: (query.data?.links ?? []).find(
      (link) => link.entity_type === "project",
    )?.entity_id,
    // An unanswered or failed read is not "no project". Falling through to
    // the deal's project would contradict the server, which re-reads the anchor.
    settled: !activityId || (!query.isPending && !query.isError),
  };
}

// What a conversation's rows are called. Colleagues come from the roster, the
// account's contacts from the record behind the drawer, whose read is cached.
export function useConversationNames(
  open: boolean,
  entityType: RelinkKind,
  entityId: string,
) {
  const viewerId = useViewerId(open);
  const roster = useRoster("user", open);
  const namesCompany = useCompany360(entityType === "company" ? entityId : "");
  const colleagues = new Map(
    (roster.data ?? []).flatMap((entry) =>
      "display_name" in entry ? [[entry.id, entry.display_name] as const] : [],
    ),
  );
  const records = recordNamesIn(namesCompany.data);
  const nameOf = (linkType: string, linkId: string) =>
    linkType === "user" ? colleagues.get(linkId) : records(linkType, linkId);
  return { viewerId, nameOf };
}

// The colleagues whose mailboxes took a thread the reader's own did not. It
// stays empty until both reads settle, because an unresolved viewer would
// make every thread read as somebody else's.
export function colleagueMailboxesOf(
  mailboxes: readonly string[] | undefined,
  viewerId: string | undefined,
): string[] {
  if (mailboxes === undefined || viewerId === undefined) {
    return [];
  }
  // The reader's own copy is not somebody else's conversation.
  if (mailboxes.includes(viewerId)) {
    return [];
  }
  return mailboxes.filter((seat) => seat !== viewerId);
}

function answeringLineOf(
  answering: string | undefined,
  anchorRead: ThreadProject,
  t: ReturnType<typeof useT>,
  locale: Locale,
): string {
  if (!answering) {
    return t("compose.newEmail");
  }
  const anchor = anchorRead.activity;
  if (!anchor?.occurred_at || anchor.content_state === "withheld") {
    return t(
      anchorRead.failed ? "compose.threadFailed" : "compose.threadPending",
    );
  }
  return t(
    anchor.direction === "outbound"
      ? "compose.followingUp"
      : "compose.replyingTo",
    {
      subject: anchor.subject || t("email.noSubject"),
      when: formatDateTime(anchor.occurred_at, locale, viewerZone()),
    },
  );
}

// Whether the thread pane rides beside the form. It holds its place while the
// anchor loads; a note is not a conversation, and a channel reply has no pane.
function conversationShown(
  read: Readonly<{
    isChannelReply: boolean;
    answering: string | undefined;
    anchorRead: ThreadProject;
    anchorUnresolved: boolean;
    conversation: ReturnType<typeof useThreadMessages>;
  }>,
): boolean {
  if (read.isChannelReply) {
    return false;
  }
  const kind = read.anchorRead.activity?.kind;
  const correspondence = kind === "email" || kind === "message";
  const { conversation } = read;
  const threadVisible =
    conversation.messages.length > 0 ||
    conversation.pending ||
    conversation.failed;
  if (correspondence && threadVisible) {
    return true;
  }
  return (
    read.anchorUnresolved || (Boolean(read.answering) && read.anchorRead.failed)
  );
}

// A reply's subject is offered once per anchor, and never over words the
// reader typed.
function useReplySubject(
  answering: string | undefined,
  anchorActivity: Activity | undefined,
  setSubject: Dispatch<SetStateAction<string>>,
) {
  const prefilledSubjects = useRef(new Set<string>());
  useEffect(() => {
    if (
      prefilledSubjects.current.has(answering ?? "") ||
      anchorActivity?.kind !== "email" ||
      !anchorActivity.subject?.trim() ||
      anchorActivity.content_state === "withheld"
    )
      return;
    prefilledSubjects.current.add(answering ?? "");
    const topic = replySubject(anchorActivity.subject);
    setSubject((current) =>
      stripEveryKeyTag(current).trim()
        ? current
        : withSubjectTag(topic, keptTag(current)),
    );
  }, [anchorActivity, answering, setSubject]);
}

// The conversation the composer answers: its anchor, its messages, the
// recent conversations offered when none is chosen, and the column layout.
export function useComposeThread({
  open,
  answering,
  isChannelReply,
  entityType,
  entityId,
  nameOf,
  setSubject,
}: Readonly<{
  open: boolean;
  answering: string | undefined;
  isChannelReply: boolean;
  entityType: RelinkKind;
  entityId: string;
  nameOf: (linkType: string, linkId: string) => string | undefined;
  setSubject: Dispatch<SetStateAction<string>>;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const anchorRead = useThreadProject(answering, open);
  const anchorActivity = anchorRead.activity;
  const conversation = useThreadMessages(open ? anchorActivity : undefined);
  // Named but not yet read: the column holds its place. A failed read is not
  // still arriving, so the pane lets go of it.
  const anchorUnresolved =
    Boolean(answering) && anchorActivity === undefined && !anchorRead.failed;
  // Mail is the transport that can open a conversation, so it alone offers.
  const offeringThreads = !isChannelReply;
  const recent = useRecentConversations(
    entityType,
    entityId,
    open && offeringThreads && !answering,
    { nameOf, t, locale },
  );
  useReplySubject(answering, anchorActivity, setSubject);
  const showConversation = conversationShown({
    isChannelReply,
    answering,
    anchorRead,
    anchorUnresolved,
    conversation,
  });
  // The ways in, when the reader has not taken one. A pending or failed read
  // keeps the column, so the record never looks as though it had no history.
  const showChoices =
    offeringThreads &&
    !answering &&
    (recent.pending || recent.failed || recent.conversations.length > 0);
  return {
    anchorRead,
    anchorActivity,
    conversation,
    anchorUnresolved,
    recent,
    answeringLine: answeringLineOf(answering, anchorRead, t, locale),
    showConversation,
    showChoices,
    splitColumns: showConversation || showChoices,
  };
}

export type ComposeThread = ReturnType<typeof useComposeThread>;
